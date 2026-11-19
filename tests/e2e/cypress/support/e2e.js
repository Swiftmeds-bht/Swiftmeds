// Shared steps for the SwiftMed end-to-end specs. Patient steps go through the
// patient app's pages, like a real patient. Test setup (stock levels, the
// expiry sweep) uses the gateway's API as the seeded pharmacist, so each test
// starts from a known state without depending on the one before.
//
// Public settings come from Cypress.expose(); the pharmacist's password and the
// worker secret only from cy.env() (cypress.config.js).

const setting = (name) => Cypress.expose(name);

const stockUrl = () => `${setting("gatewayUrl")}/api/v1/pharmacies/${setting("pharmacyId")}/stock`;

// Warm the search path once per spec file. On a freshly started stack the first
// searches can time out at the gateway (its per-try limit is shorter than
// Search's; audit finding M2, SCRUM-124), which would fail whichever test ran
// first for a reason that has nothing to do with it.
before(() => {
  const url = `${setting("gatewayUrl")}/api/v1/search?${new URLSearchParams({
    q: setting("productName"),
    lat: "27.4728",
    lng: "89.6390",
    radius_m: "5000",
  })}`;
  const attempt = (left) =>
    cy.request({ url, failOnStatusCode: false, log: false }).then(({ status }) => {
      if (status === 200) return;
      if (left === 0) throw new Error(`search still answers ${status} after warming up`);
      cy.wait(1000);
      attempt(left - 1);
    });
  attempt(10);
});

/** Signs in as the seeded pharmacist through the gateway and yields the access token. */
Cypress.Commands.add("pharmacistToken", () =>
  cy.env(["pharmacistPassword"]).then(({ pharmacistPassword }) =>
    cy
      .request({
        method: "POST",
        url: `${setting("gatewayUrl")}/api/v1/auth/login`,
        body: { email: setting("pharmacistEmail"), password: pharmacistPassword },
        log: false,
      })
      .then(({ body }) => body.tokens.accessToken),
  ),
);

/** The seeded pharmacy's stock of the test medicine, as numbers. */
Cypress.Commands.add("stockLevel", () =>
  cy.pharmacistToken().then((token) =>
    cy.request({ url: stockUrl(), headers: { Authorization: `Bearer ${token}` } }).then(({ body }) => {
      const row = (body.stock || []).find((s) => s.productId === setting("productId")) || {};
      return {
        onHand: Number(row.quantityOnHand || 0),
        reserved: Number(row.quantityReserved || 0),
        available: Number(row.quantityAvailable || 0),
      };
    }),
  ),
);

/**
 * Makes exactly `available` units sellable. Holds left ACTIVE by earlier runs
 * keep their reservation, so on-hand is set to reserved + available.
 */
Cypress.Commands.add("setAvailable", (available) =>
  cy.stockLevel().then(({ reserved }) =>
    cy.pharmacistToken().then((token) =>
      cy
        .request({
          method: "PUT",
          url: `${stockUrl()}/${setting("productId")}`,
          headers: { Authorization: `Bearer ${token}` },
          body: { quantityOnHand: reserved + available },
        })
        .then(() => cy.stockLevel().its("available").should("eq", available)),
    ),
  ),
);

/** Creates a new patient account through the sign-up page and stays signed in. */
Cypress.Commands.add("signUpPatient", () => {
  const email = `e2e-${Date.now()}-${Cypress._.random(1e6)}@e2e.swiftmeds.test`;
  const password = "e2e-password-123";
  cy.visit("/register");
  cy.get("input[name=email]").type(email);
  cy.get("input[name=password]").type(password, { log: false });
  cy.get("input[name=confirm]").type(password, { log: false });
  cy.contains("button", "Create account").click();
  cy.contains("a", "My holds").should("be.visible");
});

/** Opens the results for the test medicine near the test town. */
Cypress.Commands.add("searchForMedicine", () => {
  const q = new URLSearchParams({ q: setting("productName"), place: setting("place"), radius: "5000" });
  cy.visit(`/results?${q}`);
  cy.contains("h1", "Results");
});

/** On the results page: Reserve at the test pharmacy, choose the quantity, confirm. */
Cypress.Commands.add("reserve", (quantity) => {
  cy.contains("section.product h3", setting("productName"))
    .closest("section.product")
    .contains("li.offer", setting("pharmacyName"))
    .contains("a", "Reserve")
    .click();
  cy.contains("h1", "Reserve");
  cy.get("select[name=quantity]").select(String(quantity));
  cy.contains("button", /^Reserve$/).click();
  cy.location("pathname").should("match", /^\/holds\/[^/]+$/);
});

/** On the hold page: waits for the status (the page refreshes itself while PENDING). */
Cypress.Commands.add("holdStatus", (label) => cy.get(".hold-status").should("contain.text", label));

/** On the hold page: the pickup code as the patient would read it out. */
Cypress.Commands.add("pickupCode", () =>
  cy.get(".pickup-code").invoke("text").then((text) => text.replace(/\s/g, "")),
);

/** On the hold page: the hold's ID, from the URL. */
Cypress.Commands.add("holdId", () =>
  cy.location("pathname").then((path) => decodeURIComponent(path.split("/").pop())),
);

// Patients and the pharmacy portal are one app since SCRUM-139: one sign-in and
// one session cookie. A pharmacist step therefore saves the patient's session,
// signs in as the pharmacist on the same site, and backToPatient() puts the
// patient's session back for the rest of the test.
const SESSION_COOKIE = "swiftmed_session";
const PORTAL_HOLDS = "/pharmacy-portal/holds";
let patientCookie;

/** Saves the patient's session and signs in as the seeded pharmacist, on the portal's Holds page. */
Cypress.Commands.add("signInAsPharmacist", () =>
  cy.getCookie(SESSION_COOKIE).then((cookie) => {
    patientCookie = cookie;
    cy.clearCookie(SESSION_COOKIE);
    cy.env(["pharmacistPassword"]).then(({ pharmacistPassword }) => {
      cy.visit(`/login?next=${encodeURIComponent(PORTAL_HOLDS)}`);
      cy.get("input[name=email]").type(setting("pharmacistEmail"));
      cy.get("input[name=password]").type(pharmacistPassword, { log: false });
      cy.contains("form button", "Sign in").click();
      cy.location("pathname").should("eq", PORTAL_HOLDS);
      cy.contains("h1", "Holds");
    });
  }),
);

/** Back to the patient who was signed in before the pharmacist step. */
Cypress.Commands.add("backToPatient", () => {
  cy.clearCookie(SESSION_COOKIE);
  if (!patientCookie) return;
  const { name, value, path, domain, secure, httpOnly, sameSite } = patientCookie;
  cy.setCookie(name, value, { path, domain, secure, httpOnly, sameSite });
});

/** The pharmacist enters `code` for the hold on the portal's Holds page. */
Cypress.Commands.add("pharmacistEntersCode", (holdId, code) => {
  cy.signInAsPharmacist();
  cy.get(`[id="pickup-code-${holdId}"]`).type(code);
  cy.get(`[id="pickup-code-${holdId}"]`).closest("form").contains("button", "Collect").click();
});

/** The hold is not on the pharmacist's list of holds waiting; then back to the patient. */
Cypress.Commands.add("pharmacistDoesNotSeeHold", (holdId) => {
  cy.signInAsPharmacist();
  cy.get(`[id="pickup-code-${holdId}"]`).should("not.exist");
  cy.backToPatient();
});

/** Runs the hold-expiry sweep the Cloudflare Worker normally triggers. */
Cypress.Commands.add("runExpirySweep", () =>
  cy.env(["workerSecret"]).then(({ workerSecret }) =>
    cy.request({
      method: "POST",
      url: `${setting("gatewayUrl")}/api/v1/internal/expiry/sweep`,
      headers: { "X-Worker-Secret": workerSecret },
      body: { asOf: new Date().toISOString() },
      log: false,
    }),
  ),
);

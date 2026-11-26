// Feature: A pharmacy signs up and an administrator approves it
// (tests/features/pharmacy_onboarding.feature, SCRUM-155; the flow is
// SCRUM-141 and SCRUM-142). TC numbers are from docs/test-plan.md.
//
// Each test registers a new pharmacy at its own spot in Thimphu, so runs do
// not depend on each other. An approved test pharmacy is suspended afterwards,
// so the demo map does not fill up.

const setting = (name) => Cypress.expose(name);
const gateway = (path) => `${setting("gatewayUrl")}${path}`;

function details() {
  const stamp = `${Date.now()}-${Cypress._.random(1e6)}`;
  return {
    email: `e2e-pharmacy-${stamp}@e2e.swiftmeds.test`,
    password: "e2e-password-123",
    name: `E2E Pharmacy ${stamp}`,
    licence: `E2E-${stamp}`,
    // A spot near Thimphu no seeded pharmacy uses.
    latitude: Number((27.44 + Math.random() * 0.01).toFixed(6)),
    longitude: Number((89.66 + Math.random() * 0.01).toFixed(6)),
  };
}

/** Registers the pharmacy through /register/pharmacy, as a new owner would. */
function registerPharmacy(p) {
  cy.clearCookies();
  cy.visit("/register/pharmacy");
  cy.get("main input[name=email]").type(p.email);
  cy.get("main input[name=password]").type(p.password, { log: false });
  cy.get("main input[name=confirm]").type(p.password, { log: false });
  cy.get("main input[name=name]").type(p.name);
  cy.get("main input[name=licenseNumber]").type(p.licence);
  cy.get("main input[name=phone]").type("02334455");
  cy.get("main input[name=address]").type("Norzin Lam, Thimphu");
  cy.get("main select[name=dzongkhag]").select("Thimphu");
  // The device's location: the hidden fields the "Use my current location" button fills.
  cy.get("main input[name=latitude]").invoke("val", String(p.latitude));
  cy.get("main input[name=longitude]").invoke("val", String(p.longitude));
  cy.contains("main button", "Register pharmacy").click();
  cy.location("pathname").should("eq", "/pharmacy-portal");
}

function logIn(email, password, next) {
  cy.clearCookies();
  cy.visit(next ? `/login?next=${encodeURIComponent(next)}` : "/login");
  cy.get("main input[name=email]").type(email);
  cy.get("main input[name=password]").type(password, { log: false });
  cy.get("main form button[type=submit]").click();
}

const logInAsAdmin = () =>
  cy.env(["pharmacistPassword"]).then(({ pharmacistPassword }) => {
    logIn(setting("adminEmail"), pharmacistPassword);
    cy.location("pathname").should("eq", "/admin");
  });

/** The pharmacy's card on the admin page's "Waiting for review" tab. */
const reviewCard = (name) => {
  cy.visit("/admin?status=pending");
  return cy.contains("article.review-card", name);
};

/** Pharmacy IDs on the map within 1 km of the pharmacy's spot. */
const nearby = (p) =>
  cy
    .request(gateway(`/api/v1/pharmacies/nearby?lat=${p.latitude}&lng=${p.longitude}&radius_m=1000`))
    .then(({ body }) => (body.pharmacies || []).map((x) => x.pharmacyId));

/** The pharmacy's ID, from the admin list (the owner's session cookie is httpOnly). */
const pharmacyIdByName = (name) =>
  cy.env(["pharmacistPassword"]).then(({ pharmacistPassword }) =>
    cy
      .request("POST", gateway("/api/v1/auth/login"), { email: setting("adminEmail"), password: pharmacistPassword })
      .then(({ body }) => {
        const token = body.tokens.accessToken;
        return cy
          .request({ url: gateway("/api/v1/pharmacies?pageSize=100"), headers: { Authorization: `Bearer ${token}` } })
          .then(({ body: list }) => {
            const p = (list.pharmacies || []).find((x) => x.name === name);
            expect(p, `pharmacy ${name}`).to.exist;
            return { id: p.pharmacyId, token };
          });
      }),
  );

describe("A pharmacy signs up and an administrator approves it", () => {
  it("a new pharmacy waits for approval, then opens the portal (TC-041)", () => {
    const p = details();
    registerPharmacy(p);
    cy.contains("h1", "Waiting for approval");
    cy.contains(p.name);
    // No portal sections or holds bell until it is approved.
    cy.get("header").contains("a", "Holds").should("not.exist");

    pharmacyIdByName(p.name).then(({ id, token }) => {
      nearby(p).should("not.include", id);

      logInAsAdmin();
      reviewCard(p.name).within(() => {
        cy.contains(p.licence);
        cy.contains("button", "Approve").click();
      });
      cy.contains("[role=status]", `${p.name} was approved`);
      nearby(p).should("include", id);

      logIn(p.email, p.password);
      cy.location("pathname").should("eq", "/pharmacy-portal");
      cy.get("header nav[aria-label=Main]").within(() => {
        for (const section of ["Overview", "Holds", "Stock", "Upload CSV"]) cy.contains("a", section);
      });

      // Clean-up: off the demo map again.
      cy.request({
        method: "PUT",
        url: gateway(`/api/v1/pharmacies/${id}/verification`),
        headers: { Authorization: `Bearer ${token}` },
        body: { status: "VERIFICATION_STATUS_SUSPENDED", reason: "E2E clean-up" },
      });
    });
  });

  it("a rejected pharmacy sees why (TC-042)", () => {
    const p = details();
    registerPharmacy(p);
    cy.contains("h1", "Waiting for approval");

    logInAsAdmin();
    reviewCard(p.name).within(() => {
      cy.contains("summary", "Reject").click();
      cy.get("textarea[name=reason]").type("Licence number not found");
      cy.contains("button", "Reject pharmacy").click();
    });
    cy.contains("[role=status]", `${p.name} was rejected`);

    logIn(p.email, p.password);
    cy.location("pathname").should("eq", "/pharmacy-portal");
    cy.contains("h1", "Your registration was not approved");
    cy.contains("Licence number not found");
  });
});

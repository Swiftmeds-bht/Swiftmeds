// Feature: Reserve a medicine and collect it with a pickup code
// (tests/features/reserve_and_collect.feature, SCRUM-112). TC numbers are from
// docs/test-plan.md.

describe("Reserve a medicine and collect it with a pickup code", () => {
  beforeEach(() => {
    // Background: I am signed in as a patient, and the pharmacy has 10 to sell.
    cy.setAvailable(10);
    cy.signUpPatient();
  });

  it("reserving gives a pickup code and reserves the stock (TC-010)", () => {
    // Through the search box, as a patient would.
    cy.visit("/");
    cy.get("input[name=q]").type(Cypress.expose("productName"));
    cy.contains("button", "Search").click();
    cy.contains("h1", "Results");

    cy.reserve(2);
    cy.holdStatus("Ready to collect");
    cy.pickupCode().should("match", /^[2-9A-HJ-NP-Z]{6}$/);
    cy.get(".ticket-bottom").should("not.be.empty"); // when the hold expires
    cy.stockLevel().its("available").should("eq", 8);
  });

  it("the pharmacy hands over the medicine with the right code (TC-014)", () => {
    cy.searchForMedicine();
    cy.reserve(2);
    cy.holdStatus("Ready to collect");
    cy.location("pathname").as("holdPage");
    cy.stockLevel().then((before) => {
      cy.holdId().then((holdId) =>
        cy.pickupCode().then((code) => cy.pharmacistEntersCode(holdId, code)),
      );
      cy.origin(Cypress.expose("dashboardUrl"), () => {
        cy.contains("[role=status]", "Hold collected.");
      });

      // The patient sees it collected, and the units left the shelf for good.
      cy.get("@holdPage").then((path) => cy.visit(path));
      cy.holdStatus("Collected");
      cy.contains("You collected this medicine. Thank you.");
      cy.stockLevel().should((after) => {
        expect(after.onHand).to.eq(before.onHand - 2);
        expect(after.available).to.eq(8);
      });
    });
  });

  it("a wrong pickup code is refused (TC-014)", () => {
    cy.searchForMedicine();
    cy.reserve(2);
    cy.holdStatus("Ready to collect");
    cy.location("pathname").as("holdPage");
    cy.holdId().then((holdId) => cy.pharmacistEntersCode(holdId, "ZZZZZZ"));
    cy.origin(Cypress.expose("dashboardUrl"), () => {
      cy.contains("[role=alert]", "Wrong pickup code");
    });

    cy.get("@holdPage").then((path) => cy.visit(path));
    cy.holdStatus("Ready to collect");
    cy.stockLevel().its("available").should("eq", 8);
  });

  it("cancelling a hold gives the stock back (TC-016)", () => {
    cy.searchForMedicine();
    cy.reserve(2);
    cy.holdStatus("Ready to collect");
    cy.stockLevel().its("available").should("eq", 8);

    cy.contains("button", "Cancel hold").click();
    cy.holdStatus("Cancelled");
    cy.contains("You cancelled this hold. The units went back on sale.");
    cy.stockLevel().its("available").should("eq", 10);
  });
});

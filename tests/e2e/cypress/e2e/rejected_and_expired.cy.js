// Feature: Reservation rejected or expired
// (tests/features/reservation_rejected_and_expired.feature, SCRUM-112).

describe("Reservation rejected or expired", () => {
  beforeEach(() => {
    cy.signUpPatient();
  });

  it("is rejected when the pharmacy has too little stock left (TC-031)", () => {
    // The patient searches while 3 are left; by the time they reserve 2, only 1 is.
    cy.setAvailable(3);
    cy.searchForMedicine();
    cy.setAvailable(1);

    cy.reserve(2);
    cy.holdStatus("Not available");
    cy.contains("The pharmacy could not hold it");
    cy.get(".pickup-code").should("not.exist");
    cy.stockLevel().its("available").should("eq", 1);
  });

  it("expires when it is not collected in time, and its stock goes back on sale (TC-011, TC-015)", function () {
    const holdSeconds = Cypress.expose("holdSeconds");
    if (holdSeconds > 120) {
      // A 2-hour hold cannot be waited out in a test. tests/e2e/run.sh starts
      // the stack with a short hold time when given --start-stack.
      this.skip();
    }

    cy.setAvailable(10);
    cy.searchForMedicine();
    cy.reserve(2);
    cy.holdStatus("Ready to collect");
    cy.location("pathname").as("holdPage");
    cy.stockLevel().its("available").should("eq", 8);

    // Before the time runs out the sweep leaves the hold alone.
    cy.runExpirySweep().its("status").should("eq", 200);
    cy.reload();
    cy.holdStatus("Ready to collect");

    cy.wait((holdSeconds + 2) * 1000);
    // The sweep expires every hold past its time, including holds earlier
    // specs left active, so compare the stock before and after this sweep.
    cy.stockLevel().then((before) => {
      cy.runExpirySweep().its("status").should("eq", 200);
      cy.reload();
      cy.holdStatus("Expired");
      cy.contains("This hold ran out");
      cy.stockLevel().should((after) => {
        expect(after.onHand, "on hand").to.eq(before.onHand); // nothing left the shelf
        expect(before.reserved - after.reserved, "units released").to.be.at.least(2); // ours, at least
        expect(after.available, "back on sale").to.eq(after.onHand - after.reserved);
        expect(after.available, "back on sale").to.be.at.least(before.available + 2);
      });
    });

    // An expired hold cannot be collected: it is no longer on the pharmacist's list.
    cy.holdId().then((holdId) => cy.pharmacistDoesNotSeeHold(holdId));
  });
});

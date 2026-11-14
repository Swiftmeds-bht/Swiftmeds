# SCRUM-112. Requirements: docs/test-plan.md section 7.
@reservation
Feature: Reserve a medicine and collect it with a pickup code
  As a patient
  I want to hold a medicine at a pharmacy and get a pickup code
  So that the pharmacy keeps it for me until I arrive

  Background:
    Given I am signed in as a patient
    And "Norzin Pharmacy" is verified and has 10 packs of "Napa 500mg"

  @must @SRS-F-06 @TC-010
  Scenario: Reserving gives a pickup code and reserves the stock
    When I reserve 2 packs of "Napa 500mg" at "Norzin Pharmacy"
    Then my hold is "Ready to collect"
    And I see a 6-character pickup code
    And I see when the hold expires
    And "Norzin Pharmacy" has 8 packs of "Napa 500mg" left to sell

  @must @SRS-F-06 @SRS-F-08 @TC-014
  Scenario: The pharmacy hands over the medicine with the right code
    Given I have a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that is "Ready to collect"
    When the pharmacist enters my pickup code for that hold
    Then the hold is collected
    And I see "You collected this medicine. Thank you."
    And "Norzin Pharmacy" still has 8 packs of "Napa 500mg"

  @must @SRS-F-08 @TC-014
  Scenario: A wrong pickup code is refused
    Given I have a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that is "Ready to collect"
    When the pharmacist enters a wrong pickup code for that hold
    Then the collection is refused because the code does not match
    And my hold is still "Ready to collect"

  @must @SRS-F-08 @TC-014
  Scenario: A hold cannot be collected twice
    Given I have a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that was already collected
    When the pharmacist enters my pickup code for that hold again
    Then the collection is refused because the hold is no longer active
    And "Norzin Pharmacy" still has 8 packs of "Napa 500mg"

  @must @SRS-F-09 @TC-016
  Scenario: Cancelling a hold gives the stock back
    Given I have a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that is "Ready to collect"
    When I cancel the hold
    Then I see "You cancelled this hold. The units went back on sale."
    And "Norzin Pharmacy" has 10 packs of "Napa 500mg" left to sell

  @must @SRS-F-06 @TC-010
  Scenario Outline: Only 1 to 5 packs can be reserved at once
    When I try to reserve <quantity> packs of "Napa 500mg" at "Norzin Pharmacy"
    Then the reservation is <outcome>

    Examples:
      | quantity | outcome  |
      | 0        | refused  |
      | 1        | accepted |
      | 5        | accepted |
      | 6        | refused  |

  @must @SRS-F-06 @TC-010
  Scenario: A patient can have at most 3 open holds
    Given I already have 3 holds that are pending or ready to collect
    When I try to reserve 1 pack of "Napa 500mg" at "Norzin Pharmacy"
    Then the reservation is refused because I have too many open holds

  @must @SRS-F-07 @SRS-NFR-01 @TC-012
  Scenario: Two patients racing for the last packs cannot oversell
    Given "Changzamtog Pharmacy" is verified and has 3 packs of "Calpol 500mg"
    When another patient and I each reserve 2 packs of "Calpol 500mg" at "Changzamtog Pharmacy" at the same time
    Then exactly one of the two holds is "Ready to collect"
    And the other hold is "Not available"
    And "Changzamtog Pharmacy" has 1 pack of "Calpol 500mg" left to sell

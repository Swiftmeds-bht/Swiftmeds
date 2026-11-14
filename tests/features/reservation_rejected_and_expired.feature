# SCRUM-112. Requirements: docs/test-plan.md section 7.
@reservation
Feature: Reservation rejected or expired
  As a patient
  I want to be told clearly when a reservation cannot be kept
  So that I do not travel for a medicine that is not being held for me

  Background:
    Given I am signed in as a patient

  @must @SRS-F-06 @SRS-F-17 @TC-031
  Scenario: Reservation rejected because the pharmacy has no stock left
    Given "Norzin Pharmacy" is verified and has 1 pack of "Napa 500mg"
    When I reserve 2 packs of "Napa 500mg" at "Norzin Pharmacy"
    Then my hold becomes "Not available"
    And I see why the hold was not accepted
    And I get no pickup code
    And "Norzin Pharmacy" still has 1 pack of "Napa 500mg" left to sell

  @must @SRS-F-06 @TC-010
  Scenario: A rejected hold does not count against my open-hold limit
    Given I have 2 holds that are "Ready to collect"
    And I have 1 hold that is "Not available"
    When I reserve 1 pack of "Napa 500mg" at "Changzamtog Pharmacy"
    Then the reservation is accepted

  @must @SRS-F-09 @TC-011 @TC-015
  Scenario: An uncollected hold expires and its stock is released
    Given "Norzin Pharmacy" is verified and has 10 packs of "Napa 500mg"
    And holds last 2 hours
    And I have a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that is "Ready to collect"
    When 2 hours pass without the hold being collected
    And the expiry sweep runs
    Then my hold is "Expired"
    And "Norzin Pharmacy" has 10 packs of "Napa 500mg" left to sell

  @must @SRS-F-09 @TC-011
  Scenario: A hold just before its expiry time stays active
    Given holds last 2 hours
    And I have a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that is "Ready to collect"
    When 1 hour 59 minutes pass
    And the expiry sweep runs
    Then my hold is still "Ready to collect"

  @must @SRS-F-08 @SRS-F-09 @TC-014 @TC-015
  Scenario: An expired hold cannot be collected
    Given I have a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that has expired
    When the pharmacist enters my pickup code for that hold
    Then the collection is refused because the hold is no longer active

  @must @SRS-F-07 @SRS-F-17 @TC-013
  Scenario: The same stock answer delivered twice changes nothing
    Given "Norzin Pharmacy" is verified and has 10 packs of "Napa 500mg"
    And I reserved 2 packs of "Napa 500mg" at "Norzin Pharmacy"
    When Inventory's "stock reserved" answer for my hold is delivered twice
    Then I have exactly one hold, and it is "Ready to collect"
    And "Norzin Pharmacy" has 8 packs of "Napa 500mg" left to sell

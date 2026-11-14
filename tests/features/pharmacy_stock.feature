# SCRUM-112. Requirements: docs/test-plan.md section 7.
@pharmacy
Feature: A pharmacy keeps its stock up to date
  As pharmacy staff
  I want to update my stock from the dashboard
  So that patients searching nearby see what I really have

  Background:
    Given "Norzin Pharmacy" is verified
    And I am signed in to the pharmacy dashboard as staff of "Norzin Pharmacy"

  @must @SRS-F-10 @SRS-F-14 @TC-017 @TC-018 @TC-026
  Scenario: A stock update shows up in patient search
    Given "Norzin Pharmacy" has 0 packs of "Napa 500mg"
    When I set the on-hand quantity of "Napa 500mg" to 15
    Then the dashboard shows 15 packs of "Napa 500mg" available
    And a patient searching for "Napa 500mg" within 5 km of Thimphu sees "Norzin Pharmacy" with "In stock"

  @must @SRS-F-10 @TC-017
  Scenario: Reserved packs are protected when stock is updated
    Given "Norzin Pharmacy" has 10 packs of "Napa 500mg"
    And patients have reserved 4 of them
    When I try to set the on-hand quantity of "Napa 500mg" to 3
    Then the update is refused because 4 packs are reserved
    And the dashboard still shows 10 on hand and 4 reserved

  @must @SRS-F-10 @TC-017
  Scenario: Importing a stock CSV reports the rows it rejected
    When I upload a stock CSV with 3 valid rows and 1 row with a negative quantity
    Then the import report shows 3 rows imported and 1 row rejected
    And the rejected row shows its reason

  @must @SRS-F-08 @TC-014
  Scenario: Staff collect a patient's hold from the dashboard
    Given a patient has a hold for 2 packs of "Napa 500mg" at "Norzin Pharmacy" that is "Ready to collect"
    When I enter the patient's pickup code under "Collect hold"
    Then the hold disappears from "Active holds"
    And the patient's hold shows as collected

  @must @SRS-F-14 @TC-025
  Scenario: An unverified pharmacy's stock is not shown to patients
    Given "Motithang Pharmacy" is waiting for verification
    And "Motithang Pharmacy" has 9 packs of "Napa 500mg"
    When a patient searches for "Napa 500mg" within 5 km of Thimphu
    Then the patient does not see "Motithang Pharmacy" in the results

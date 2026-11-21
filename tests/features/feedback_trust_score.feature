# SCRUM-112. Requirements: docs/test-plan.md section 7.
@feedback
Feature: Feedback after pickup updates the pharmacy's trust score
  As a patient
  I want to say whether the medicine was really in stock when I arrived
  So that pharmacies whose stock figures are wrong rank lower for everyone

  Background:
    Given I am signed in as a patient
    And "Norzin Pharmacy" has no feedback yet, so its trust score is 0.5

  @should @SRS-F-18 @TC-033
  Scenario: "It was in stock" raises the trust score
    Given I collected a hold at "Norzin Pharmacy"
    When I give feedback that the medicine was in stock, with rating 5
    Then the feedback is recorded
    And the trust score of "Norzin Pharmacy" goes up to 0.625

  @should @SRS-F-18 @TC-033
  Scenario: "It was not in stock" lowers the trust score
    Given I collected a hold at "Norzin Pharmacy"
    When I give feedback that the medicine was not in stock
    Then the feedback is recorded
    And the trust score of "Norzin Pharmacy" goes down to 0.375

  @should @SRS-F-18 @SRS-F-04 @TC-033 @TC-007
  Scenario: A higher trust score moves a pharmacy up in search results
    Given "Norzin Pharmacy" and "Changzamtog Pharmacy" are the same in distance, freshness and price for "Napa 500mg"
    And patients reported that "Changzamtog Pharmacy" had the medicine 5 times
    And patients reported that "Norzin Pharmacy" did not have it 5 times
    When I search for "Napa 500mg" within 5 km of Thimphu
    Then "Changzamtog Pharmacy" is ranked above "Norzin Pharmacy"

  @should @SRS-F-18 @TC-033
  Scenario: The rating alone does not change the trust score
    Given I collected a hold at "Norzin Pharmacy"
    When I give feedback that the medicine was in stock, with rating 1
    Then the trust score of "Norzin Pharmacy" goes up to 0.625

  @should @SRS-F-18 @TC-033
  Scenario: Only one feedback per hold
    Given I collected a hold at "Norzin Pharmacy"
    And I already gave feedback for that hold
    When I give feedback for the same hold again
    Then the feedback is refused because it was already given
    And the trust score of "Norzin Pharmacy" does not change

  @should @SRS-F-18 @SRS-F-15 @TC-033 @TC-028
  Scenario Outline: Feedback is only accepted for my own collected hold
    Given <hold>
    When I give feedback that the medicine was not in stock for that hold
    Then the feedback is refused because <reason>
    And the trust score of "Norzin Pharmacy" stays 0.5

    Examples:
      | hold                                                           | reason                        |
      | I have a hold at "Norzin Pharmacy" that is "Ready to collect"  | the hold was not collected    |
      | another patient collected a hold at "Norzin Pharmacy"          | it is not my hold             |

  @should @SRS-F-18 @TC-033
  Scenario Outline: Rating and comment are checked
    Given I collected a hold at "Norzin Pharmacy"
    When I give feedback with rating <rating> and a comment of <length> characters
    Then the feedback is <outcome>

    Examples:
      | rating | length | outcome  |
      | 0      | 0      | accepted |
      | 5      | 500    | accepted |
      | 6      | 0      | refused  |
      | 3      | 501    | refused  |

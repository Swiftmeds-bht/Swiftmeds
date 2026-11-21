# SCRUM-112. Requirements: docs/test-plan.md section 7.
@auth
Feature: Patients and pharmacy staff only reach their own features
  As a patient or a member of pharmacy staff
  I want my account and my data to be protected
  So that nobody else can see or change my holds or my pharmacy's stock

  @must @SRS-F-15 @TC-027
  Scenario: A patient registers and signs in
    When I create a patient account with a valid email and an 8-character password
    And I sign in with that email and password
    Then I see "My holds"

  @must @SRS-F-15 @TC-027
  Scenario Outline: Passwords must be 8 to 72 characters long
    When I try to create a patient account with a <length>-character password
    Then the account is <outcome>

    Examples:
      | length | outcome  |
      | 7      | refused  |
      | 8      | created  |
      | 72     | created  |
      | 73     | refused  |

  @must @SRS-F-15 @TC-027
  Scenario: Reserving needs a signed-in patient
    Given I am not signed in
    When I try to reserve 1 pack of "Napa 500mg" at "Norzin Pharmacy"
    Then I am sent to sign in
    And I see "Sign in to reserve medicine and see your holds."

  @must @SRS-F-15 @TC-027
  Scenario: An expired session asks me to sign in again
    Given I am signed in as a patient
    And my session has ended
    When I open "My holds"
    Then I see "Your session ended. Please sign in again."

  @must @SRS-F-15 @TC-028
  Scenario: A patient cannot change pharmacy stock
    Given I am signed in as a patient
    When I try to update the stock of "Norzin Pharmacy"
    Then the request is refused as forbidden

  @must @SRS-F-15 @TC-028
  Scenario: A patient cannot see another patient's hold
    Given I am signed in as a patient
    And another patient has a hold at "Norzin Pharmacy"
    When I open that hold
    Then the hold is not shown to me

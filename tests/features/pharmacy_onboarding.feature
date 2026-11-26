# SCRUM-155 (covers SCRUM-141 and SCRUM-142).
@onboarding
Feature: A pharmacy signs up and an administrator approves it
  As a pharmacy owner
  I want to register my pharmacy on SwiftMed
  So that patients can find my stock once SwiftMed has checked my licence

  @must @TC-030
  Scenario: A new pharmacy waits for approval, then opens the portal
    Given I am not signed in
    When I register "Thimphu Care" with its licence number, phone, address and dzongkhag
    Then I see "Waiting for approval"
    And the portal's sections are not shown yet
    And "Thimphu Care" is not on the map
    When an administrator approves "Thimphu Care" on the admin page
    Then the administrator sees "Thimphu Care was approved"
    And "Thimphu Care" is on the map
    When I log in to the pharmacy portal
    Then I see the portal's Overview, Holds, Stock and Upload CSV

  @must @TC-031
  Scenario: A rejected pharmacy sees why
    Given "Wrong Licence Pharmacy" has registered and is waiting for approval
    When an administrator rejects it with the reason "Licence number not found"
    And the pharmacy logs in to the pharmacy portal
    Then it sees "Your registration was not approved"
    And it sees the reason "Licence number not found"

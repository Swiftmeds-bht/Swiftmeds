# SCRUM-112. Requirements: docs/test-plan.md section 7.
@watchlist
Feature: Watch a medicine and get an alert when it is back in stock
  As a patient
  I want to be told when a medicine I could not find comes back near me
  So that I do not have to keep searching for it

  Background:
    Given I am signed in as a patient
    And no pharmacy within 5 km of Thimphu has "Amoxil 250mg"

  @should @SRS-F-13 @TC-023
  Scenario: Subscribing from an empty search
    When I search for "Amoxil 250mg" within 5 km of Thimphu
    And I press "Notify me when it is back"
    Then my watchlist shows "Amoxil 250mg" as "Waiting"

  @should @SRS-F-13 @TC-023
  Scenario: Subscribing twice to the same medicine keeps one subscription
    Given I am watching "Amoxil 250mg" within 5 km of Thimphu
    When I press "Notify me when it is back" for "Amoxil 250mg" again
    Then my watchlist has exactly one entry for "Amoxil 250mg"

  @should @SRS-F-13 @TC-024
  Scenario: Restock inside my area sends one alert
    Given I am watching "Amoxil 250mg" within 5 km of Thimphu
    When "Norzin Pharmacy", 800 m away, adds 20 packs of "Amoxil 250mg"
    Then I get the alert "Amoxil 250mg is back in stock"
    And my watchlist shows "Amoxil 250mg" as "Back in stock"

  @should @SRS-F-13 @TC-024
  Scenario: Restock outside my area sends no alert
    Given I am watching "Amoxil 250mg" within 5 km of Thimphu
    When a pharmacy in Paro, 50 km away, adds 20 packs of "Amoxil 250mg"
    Then I get no alert
    And my watchlist still shows "Amoxil 250mg" as "Waiting"

  @should @SRS-F-13 @TC-024
  Scenario: A second restock does not alert me again
    Given I am watching "Amoxil 250mg" within 5 km of Thimphu
    And I already got the alert for "Amoxil 250mg"
    When "Changzamtog Pharmacy" also adds 10 packs of "Amoxil 250mg"
    Then I get no new alert

  @should @SRS-F-13 @TC-024
  Scenario: An alert raised while I was away arrives when I open the app
    Given I am watching "Amoxil 250mg" within 5 km of Thimphu
    And I have closed the app
    When "Norzin Pharmacy" adds 20 packs of "Amoxil 250mg"
    And I open the app again
    Then I get the alert "Amoxil 250mg is back in stock" once

  @should @SRS-F-13 @TC-023
  Scenario: Removing a medicine from the watchlist
    Given I am watching "Amoxil 250mg" within 5 km of Thimphu
    When I remove "Amoxil 250mg" from my watchlist
    Then I see "You are not watching any medicine."

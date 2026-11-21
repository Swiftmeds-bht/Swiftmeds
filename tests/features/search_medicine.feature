# SCRUM-112. Requirements: docs/test-plan.md section 7.
@search
Feature: Search for a medicine nearby
  As a patient
  I want to find which pharmacies near me have my medicine in stock
  So that I only travel to a pharmacy that can actually give it to me

  Background:
    Given these verified pharmacies in Thimphu:
      | pharmacy             | distance_m |
      | Norzin Pharmacy      | 800        |
      | Changzamtog Pharmacy | 2400       |
    And the catalogue has these products:
      | product           | molecule    | strength | form   |
      | Napa 500mg        | paracetamol | 500 mg   | tablet |
      | Calpol 500mg      | paracetamol | 500 mg   | tablet |
      | Panadol 1000mg    | paracetamol | 1000 mg  | tablet |
      | Amoxil 250mg      | amoxicillin | 250 mg   | capsule |

  @must @SRS-F-01 @SRS-F-02 @SRS-F-05 @TC-001 @TC-003 @TC-009
  Scenario: Medicine in stock at a nearby pharmacy
    Given "Norzin Pharmacy" has 12 packs of "Napa 500mg" at Nu. 35 each
    And the stock was confirmed 1 hour ago
    When I search for "Napa 500mg" within 5 km of Thimphu
    Then I see "Norzin Pharmacy" in the results
    And the result shows "In stock", the price Nu. 35 and a distance of 800 m
    And the result shows a high freshness label

  @must @SRS-F-01 @TC-002
  Scenario: Searching by molecule finds every brand with that molecule
    Given "Norzin Pharmacy" has 5 packs of "Napa 500mg"
    And "Changzamtog Pharmacy" has 3 packs of "Calpol 500mg"
    When I search for "paracetamol 500 mg" within 5 km of Thimphu
    Then the results include "Napa 500mg" at "Norzin Pharmacy"
    And the results include "Calpol 500mg" at "Changzamtog Pharmacy"
    And the results do not include "Amoxil 250mg"

  @must @SRS-F-02 @TC-003
  Scenario: Pharmacies with no stock or no verification are not listed
    Given "Norzin Pharmacy" has 0 packs of "Napa 500mg"
    And "Changzamtog Pharmacy" has 4 packs of "Napa 500mg"
    And "Motithang Pharmacy" is waiting for verification and has 9 packs of "Napa 500mg"
    When I search for "Napa 500mg" within 5 km of Thimphu
    Then I see "Changzamtog Pharmacy" in the results
    And I do not see "Norzin Pharmacy" in the results
    And I do not see "Motithang Pharmacy" in the results

  @must @SRS-F-04 @TC-007
  Scenario: The nearest pharmacy with fresh stock is ranked first
    Given "Norzin Pharmacy" has 5 packs of "Napa 500mg" confirmed 1 hour ago
    And "Changzamtog Pharmacy" has 5 packs of "Napa 500mg" confirmed 1 hour ago
    And both pharmacies sell "Napa 500mg" at Nu. 35 and have the same trust score
    When I search for "Napa 500mg" within 5 km of Thimphu
    Then "Norzin Pharmacy" is ranked above "Changzamtog Pharmacy"

  @must @SRS-F-11 @TC-020
  Scenario: Stock that was not confirmed for a long time is labelled
    Given "Norzin Pharmacy" has 5 packs of "Napa 500mg" confirmed 30 hours ago
    When I search for "Napa 500mg" within 5 km of Thimphu
    Then the result for "Norzin Pharmacy" shows a low freshness label

  @should @SRS-F-12 @TC-021
  Scenario: Out of stock everywhere, a substitute with the same molecule and strength is offered
    Given no pharmacy within 5 km has "Napa 500mg"
    And "Changzamtog Pharmacy" has 6 packs of "Calpol 500mg"
    When I search for "Napa 500mg" within 5 km of Thimphu
    Then I see "Not in stock at any pharmacy in this area."
    And under "Same medicine, other brands" I see "Calpol 500mg" at "Changzamtog Pharmacy"
    And "Panadol 1000mg" is not offered as an exact substitute

  @should @SRS-F-12 @TC-022
  Scenario: Out of stock and no substitute exists
    Given no pharmacy within 5 km has "Amoxil 250mg"
    And no other product contains amoxicillin
    When I search for "Amoxil 250mg" within 5 km of Thimphu
    Then I see "Not in stock at any pharmacy in this area."
    And no substitute is offered
    And I can press "Notify me when it is back"

  @must @SRS-F-03 @SRS-F-16 @TC-005 @TC-006
  Scenario Outline: The search radius is checked
    When I search for "Napa 500mg" with a radius of <radius_m> m
    Then the search is <outcome>

    Examples:
      | radius_m | outcome                  |
      | 0        | run with 5000 m          |
      | 499      | refused as invalid input |
      | 500      | run with 500 m           |
      | 50000    | run with 50000 m         |
      | 50001    | refused as invalid input |

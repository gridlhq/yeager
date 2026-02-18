@p1 @status
Feature: Status Command Depth
  As a developer using Yeager CLI
  I want accurate status information in all states
  So that I can debug issues and understand VM state

  Scenario: Status JSON schema validation
    Given the shared project directory
    And the VM is running
    When I run "yg status --json"
    Then the exit code should be 0
    And the output should be valid JSON
    And the JSON output should have field "state"
    And the JSON output should have field "instance_id"
    And the JSON output should have field "region"

  Scenario: Status during VM transition states
    Given a temporary project directory
    And a Go project in the project directory
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg status"
    Then the exit code should be 0
    And the output should contain one of:
      | State: running |
      | running        |
      | State: active  |
      | active         |

  Scenario: Status with corrupted state file
    Given the shared project directory
    And the VM is running
    When I corrupt the state file at ".yeager/state.json"
    And I run "yg status"
    Then the exit code should be 0
    And the output should contain one of:
      | syncing from AWS |
      | reconciling      |
      | recovering       |
      | running          |

  Scenario: Status with no VM provisioned (fresh init)
    Given a temporary project directory
    And a Go project in the project directory
    When I run "yg status"
    Then the exit code should be 0
    And the output should contain one of:
      | no VM found            |
      | not initialized        |
      | No VM provisioned      |
      | Run 'yg up'            |

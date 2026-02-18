@livefire
Feature: Kill running commands
  As a developer who needs to cancel a running command
  I want to kill commands on the remote VM
  So that I can stop long-running or stuck processes

  Background:
    Given the shared project directory
    And the VM is running

  Scenario: Kill attempts to cancel the last command
    Given I have run "yg echo kill-test-marker"
    When I run "yg kill" with a 60 second timeout
    Then the output should contain "cancelling"

  Scenario: Kill with explicit run ID
    Given I have run "yg echo kill-by-id-marker"
    And I capture the last run ID from status
    When I run kill with the captured run ID
    Then the output should contain "cancelling"

@livefire
Feature: Run logs and history
  As a developer who has run commands on a remote VM
  I want to review output and track my run history
  So that I can debug issues and see what happened

  Background:
    Given the shared project directory
    And the VM is running

  Scenario: Logs replays output from the last run
    Given I have run "yg echo logs-marker-12345"
    When I run "yg logs" with a 60 second timeout
    Then the exit code should be 0

  Scenario: Logs with --tail limits output lines
    Given I have run "yg echo tail-test-line"
    When I run "yg logs --tail 5" with a 60 second timeout
    Then the exit code should be 0

  Scenario: Logs with explicit run ID replays that run
    Given I have run "yg echo runid-marker-xyz"
    And I capture the last run ID from status
    When I run logs with the captured run ID
    Then the exit code should be 0

  Scenario: Status shows recent run history
    When I run "yg status" with a 30 second timeout
    Then the exit code should be 0
    And the output should contain "running"
    And the output should contain "recent runs:"

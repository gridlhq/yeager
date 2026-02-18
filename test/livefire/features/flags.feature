@livefire
Feature: Global CLI flags
  As a developer integrating yeager into scripts and workflows
  I want global flags to control output format and verbosity
  So that I can parse output programmatically or debug issues

  Background:
    Given the shared project directory
    And the VM is running

  Scenario: --quiet suppresses yeager status messages
    When I run "yg --quiet echo quiet-marker" with a 180 second timeout
    Then the exit code should be 0
    And the output should contain "quiet-marker"
    And the output should not contain "yeager |"
    And the output should not contain "syncing"

  Scenario: --json status returns valid JSON
    When I run "yg --json status" with a 30 second timeout
    Then the exit code should be 0
    And the output should be valid JSON lines

  Scenario: --verbose enables debug output
    When I run "yg -v status" with a 30 second timeout
    Then the exit code should be 0

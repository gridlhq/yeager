@livefire
Feature: Remote command execution
  As a developer using yeager to run commands remotely
  I want commands to execute on the VM and stream output back
  So that I get the same experience as running locally

  Background:
    Given the shared project directory

  Scenario: First command creates a VM and executes successfully
    When I run "yg echo hello-livefire" with a 600 second timeout
    Then the exit code should be 0
    And the output should contain "hello-livefire"
    And the output should contain "launching"
    And the output should contain "VM ready"
    And the output should contain "syncing files"

  Scenario: Subsequent command reuses the existing VM
    When I run "yg echo reuse-check" with a 180 second timeout
    Then the exit code should be 0
    And the output should contain "reuse-check"
    And the output should contain "VM running"
    And the output should not contain "launching"
    And the output should not contain "creating"

  Scenario: pwd shows the remote project directory
    When I run "yg pwd" with a 180 second timeout
    Then the exit code should be 0
    And the output should contain "/home/ubuntu/project"

  Scenario: Failed command propagates non-zero exit code
    When I run "yg false" with a 180 second timeout
    Then the exit code should not be 0

  Scenario: Command with multiple arguments preserves them
    When I run "yg echo one two three" with a 180 second timeout
    Then the exit code should be 0
    And the output should contain "one two three"

  Scenario: Command with flags passes them through
    When I run "yg ls -la /tmp" with a 180 second timeout
    Then the exit code should be 0

  Scenario: Command shows exit code and duration on completion
    When I run "yg echo timing-check" with a 180 second timeout
    Then the exit code should be 0
    And the output should contain "done (exit 0)"

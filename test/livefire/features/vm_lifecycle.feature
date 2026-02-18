@livefire @lifecycle
Feature: VM lifecycle management
  As a developer managing cloud costs
  I want full control over the VM lifecycle
  So that I can stop, restart, and destroy VMs on demand

  Background:
    Given the shared project directory

  Scenario: Stop command halts the running VM
    Given the VM is running
    When I run "yg stop" with a 120 second timeout
    Then the exit code should be 0

  Scenario: VM reaches stopped state after stop command
    Given I wait for VM status to contain "stopped" within 120 seconds

  Scenario: Up command restarts a stopped VM
    When I run "yg up" with a 300 second timeout
    Then the exit code should be 0
    And the output should contain "VM running"

  Scenario: Command succeeds on a restarted VM
    When I run "yg echo after-restart" with a 180 second timeout
    Then the exit code should be 0
    And the output should contain "after-restart"

  Scenario: Destroy terminates the VM completely
    When I run "yg destroy --force" with a 120 second timeout
    Then the exit code should be 0

  Scenario: Status after destroy shows no VM
    When I run "yg status" with a 30 second timeout
    Then the exit code should be 0
    And the output should contain "no VM"

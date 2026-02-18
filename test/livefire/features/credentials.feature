@offline @credentials
Feature: AWS credential error handling
  As a developer with misconfigured AWS credentials
  I want clear, actionable error messages
  So that I can quickly diagnose and fix my configuration

  Scenario: Invalid AWS credentials produce a visible error
    Given a temporary project directory
    And a Go project in the project directory
    And the AWS credentials are set to invalid values
    When I run "yg echo hello"
    Then the exit code should not be 0
    And the output should not be empty
    And the output should contain "credentials"
    And the output should contain "invalid"

  Scenario: Missing AWS credentials suggest how to configure
    Given a temporary project directory
    And a Go project in the project directory
    And all AWS credential sources are removed
    When I run "yg echo hello"
    Then the exit code should not be 0
    And the output should not be empty
    And the output should contain "credentials"
    And the output should contain "yg configure"

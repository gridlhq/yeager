@offline
Feature: CLI smoke tests
  As a developer installing yeager for the first time
  I want basic CLI commands to work correctly
  So that I can verify my installation before connecting to AWS

  Scenario: No arguments displays help with grouped commands
    Given a temporary project directory
    When I run "yg"
    Then the exit code should be 0
    And the output should contain "Commands:"
    And the output should contain "Setup:"

  Scenario: --help flag displays help with all subcommands
    Given a temporary project directory
    When I run "yg --help"
    Then the exit code should be 0
    And the output should contain "configure"
    And the output should contain "status"
    And the output should contain "logs"
    And the output should contain "kill"
    And the output should contain "stop"
    And the output should contain "destroy"
    And the output should contain "init"
    And the output should contain "up"

  Scenario: --version displays version string
    Given a temporary project directory
    When I run "yg --version"
    Then the exit code should be 0
    And the output should contain "yg version"

  Scenario: Init creates .yeager.toml config file
    Given a temporary project directory
    When I run "yg init"
    Then the exit code should be 0
    And the output should contain ".yeager.toml"
    And the file ".yeager.toml" should exist in the project directory

  Scenario: Init warns when config already exists
    Given a temporary project directory
    And a ".yeager.toml" file exists in the project directory
    When I run "yg init"
    Then the output should contain "already"

  Scenario: Init --force overwrites existing config
    Given a temporary project directory
    And a ".yeager.toml" file exists in the project directory
    When I run "yg init --force"
    Then the exit code should be 0
    And the file ".yeager.toml" should exist in the project directory

  Scenario: Configure help describes credential setup
    Given a temporary project directory
    When I run "yg configure --help"
    Then the exit code should be 0
    And the output should contain "aws-access-key-id"
    And the output should contain "aws-secret-access-key"
    And the output should contain "profile"

  Scenario: Kill help describes command cancellation
    Given a temporary project directory
    When I run "yg kill --help"
    Then the exit code should be 0
    And the output should contain "cancel"
    And the output should contain "kill"

  Scenario: Status with no VM shows helpful message
    Given a temporary project directory
    And a Go project in the project directory
    When I run "yg status"
    Then the exit code should be 0
    And the output should contain "no VM found"

  Scenario Outline: Subcommand help is available for <command>
    Given a temporary project directory
    When I run "yg <command> --help"
    Then the exit code should be 0

    Examples:
      | command   |
      | configure |
      | status    |
      | logs      |
      | kill      |
      | stop      |
      | destroy   |
      | init      |
      | up        |

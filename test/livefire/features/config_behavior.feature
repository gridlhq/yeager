@config @p0
Feature: Configuration File Behavior
  As a Yeager user editing .yeager.toml
  I want my config changes to control VM behavior
  So that I can customize my development environment

  Background:
    Given the shared project directory

  # ─────────────────────────────────────────────────────────────────────────
  # [compute] section - VM sizing
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Compute size "small" provisions correct instance type
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [compute]
      size = "small"
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg status --json"
    Then the exit code should be 0
    And the JSON output field ".instance_type" should match "(t3\\.small|t3a\\.small)"

  Scenario: Compute size "xlarge" provisions correct instance type
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [compute]
      size = "xlarge"
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg status --json"
    Then the exit code should be 0
    And the JSON output field ".instance_type" should match "(c5\\.2xlarge|c6i\\.2xlarge|c5n\\.2xlarge)"

  Scenario: Custom region in config is used
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [compute]
      region = "eu-west-1"
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg status --json"
    Then the exit code should be 0
    And the JSON output field ".availability_zone" should start with "eu-west-1"

  # ─────────────────────────────────────────────────────────────────────────
  # [lifecycle] section - Auto-stop behavior
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: idle_stop enabled stops VM after grace period
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [lifecycle]
      idle_stop = true
      grace_period = "90s"
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run echo test" with a 120 second timeout
    Then the exit code should be 0
    When I wait 120 seconds
    And I run "yg status --json"
    Then the JSON output field ".state" should be "stopped"

  Scenario: idle_stop disabled keeps VM running
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [lifecycle]
      idle_stop = false
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run echo test" with a 120 second timeout
    Then the exit code should be 0
    When I wait 150 seconds
    And I run "yg status --json"
    Then the JSON output field ".state" should be "running"

  # ─────────────────────────────────────────────────────────────────────────
  # [setup] section - Initial provisioning
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Setup packages are installed on first boot
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [setup]
      packages = ["jq", "htop"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run which jq" with a 120 second timeout
    Then the exit code should be 0
    And the output should contain "/usr/bin/jq"
    When I run "yg run which htop" with a 120 second timeout
    Then the exit code should be 0
    And the output should contain "/usr/bin/htop"

  Scenario: Setup run commands execute on provision
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [setup]
      run = ["echo HELLO > /tmp/test.txt", "mkdir -p /tmp/workspace"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run cat /tmp/test.txt" with a 120 second timeout
    Then the exit code should be 0
    And the output should contain "HELLO"
    When I run "yg run ls -d /tmp/workspace" with a 120 second timeout
    Then the exit code should be 0

  # ─────────────────────────────────────────────────────────────────────────
  # [sync] section - File transfer rules
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Sync include pattern overrides defaults
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [sync]
      include = ["*.go", "*.md"]
      """
    And the file "test.py" exists with content "print('test')"
    And the file "README.md" exists with content "# README"
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run test -f main.go" with a 120 second timeout
    Then the exit code should be 0
    When I run "yg run test -f README.md" with a 120 second timeout
    Then the exit code should be 0
    When I run "yg run test -f test.py" with a 120 second timeout
    Then the exit code should not be 0

  Scenario: Sync exclude pattern prevents upload
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [sync]
      exclude = ["node_modules/", "*.log"]
      """
    And the directory "node_modules" exists
    And the file "debug.log" exists with content "debug log"
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run test -d node_modules" with a 120 second timeout
    Then the exit code should not be 0
    When I run "yg run test -f debug.log" with a 120 second timeout
    Then the exit code should not be 0

  # ─────────────────────────────────────────────────────────────────────────
  # [artifacts] section - Output collection
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Artifacts path collects files after command
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [artifacts]
      path = ["dist/", "*.zip"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run mkdir dist && echo BUILT > dist/app.txt" with a 120 second timeout
    Then the exit code should be 0
    When I run "yg run echo ZIP > output.zip" with a 120 second timeout
    Then the exit code should be 0
    Then the local file "artifacts/dist/app.txt" should exist
    And the local file "artifacts/dist/app.txt" should contain "BUILT"
    And the local file "artifacts/output.zip" should exist
    And the local file "artifacts/output.zip" should contain "ZIP"

  Scenario: Default config with no lifecycle section allows VM to run
    Given a temporary project directory
    And a Go project in the project directory
    When I run "yg init"
    Then the exit code should be 0
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run echo test" with a 120 second timeout
    Then the exit code should be 0
    When I wait 30 seconds
    And I run "yg status --json"
    Then the JSON output field ".state" should be "running"

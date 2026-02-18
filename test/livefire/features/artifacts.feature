@artifacts @p0
Feature: Artifact Collection
  As a Yeager user running build/CI workflows
  I want artifacts to be automatically downloaded after commands
  So that I can access build outputs without manual file transfers

  Background:
    Given the shared project directory

  # ─────────────────────────────────────────────────────────────────────────
  # Single file artifacts
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Single file artifact download
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [artifacts]
      path = ["output.txt"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run echo RESULT > output.txt" with a 120 second timeout
    Then the exit code should be 0
    And the local file "artifacts/output.txt" should exist
    And the local file "artifacts/output.txt" should contain "RESULT"

  # ─────────────────────────────────────────────────────────────────────────
  # Directory artifacts
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Directory artifact download
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [artifacts]
      path = ["dist/"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run mkdir -p dist && echo OK > dist/file.txt && echo TWO > dist/other.txt" with a 120 second timeout
    Then the exit code should be 0
    And the local file "artifacts/dist/file.txt" should exist
    And the local file "artifacts/dist/file.txt" should contain "OK"
    And the local file "artifacts/dist/other.txt" should exist
    And the local file "artifacts/dist/other.txt" should contain "TWO"

  # ─────────────────────────────────────────────────────────────────────────
  # Glob pattern artifacts
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Glob pattern artifact matching
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [artifacts]
      path = ["**/*.log"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run mkdir -p a/b && echo LOG1 > a/test.log && echo LOG2 > a/b/debug.log" with a 120 second timeout
    Then the exit code should be 0
    And the local file "artifacts/a/test.log" should exist
    And the local file "artifacts/a/test.log" should contain "LOG1"
    And the local file "artifacts/a/b/debug.log" should exist
    And the local file "artifacts/a/b/debug.log" should contain "LOG2"

  # ─────────────────────────────────────────────────────────────────────────
  # Large file handling
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Large artifact handling
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [artifacts]
      path = ["large.bin"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run dd if=/dev/zero of=large.bin bs=1M count=100" with a 180 second timeout
    Then the exit code should be 0
    And the local file "artifacts/large.bin" should exist
    And the local file "artifacts/large.bin" should have size 104857600

  # ─────────────────────────────────────────────────────────────────────────
  # Missing artifact handling
  # ─────────────────────────────────────────────────────────────────────────

  Scenario: Missing artifact path shows warnings
    Given a temporary project directory
    And a Go project in the project directory
    And the config file contains:
      """
      [artifacts]
      path = ["nonexistent.txt", "also-missing.zip"]
      """
    When I run "yg up" with a 600 second timeout
    Then the exit code should be 0
    When I run "yg run echo done" with a 120 second timeout
    Then the exit code should be 0
    And the output should contain one of:
      | text                |
      | warning             |
      | not found           |
      | missing             |
      | could not find      |
      | does not exist      |

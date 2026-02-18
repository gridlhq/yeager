@p1 @sync
Feature: File Sync Edge Cases
  As a developer using Yeager CLI
  I want files to sync reliably with edge cases handled
  So that large files, symlinks, permissions, and special filenames work correctly

  Background:
    Given the shared project directory
    And the VM is running

  Scenario: Large file sync (500MB)
    Given I create a 500MB file named "large.bin"
    When I run "yg run 'ls -lh large.bin && du -sh large.bin'"
    Then the exit code should be 0
    And the output should contain "large.bin"
    When I run "yg run 'stat -c %s large.bin'"
    Then the output should match pattern "524288000"

  Scenario: .gitignore exclusions work
    Given I create a ".gitignore" file containing "*.tmp" and "build/"
    And I create file "test.tmp" with content "SHOULD_NOT_SYNC"
    And I create file "app.go" with content "package main"
    And I create file "build/output" with content "BUILD_ARTIFACT"
    When I run "yg run 'ls -la && echo --- && cat app.go 2>&1 || true && echo --- && cat test.tmp 2>&1 || true && echo --- && ls build/ 2>&1 || true'"
    Then the exit code should be 0
    And the output should contain "package main"
    And the output should not contain "SHOULD_NOT_SYNC"
    And the output should not contain "BUILD_ARTIFACT"

  Scenario: Symlink handling
    Given I create file "target.txt" with content "TARGET_CONTENT"
    And I create symlink "link.txt" pointing to "target.txt"
    When I run "yg run 'readlink link.txt || ls -la link.txt'"
    Then the exit code should be 0
    When I run "yg run 'cat link.txt'"
    Then the exit code should be 0
    And the output should contain "TARGET_CONTENT"

  Scenario: Permission preservation
    Given I create file "script.sh" with mode 755
    And I create file "readonly.txt" with mode 444
    When I run "yg run 'stat -c %a script.sh'"
    Then the exit code should be 0
    And the output should contain "755"
    When I run "yg run 'stat -c %a readonly.txt'"
    Then the exit code should be 0
    And the output should contain "444"

  Scenario: Binary file sync (no corruption)
    Given I create a binary file "app.bin"
    And I compute checksum of "app.bin" as "local_checksum"
    When I run "yg run 'sha256sum app.bin'"
    Then the exit code should be 0
    And the output should contain the stored checksum

  Scenario: UTF-8 filename handling
    Given I create file "测试.txt" with content "Chinese"
    And I create file "émoji🚀.md" with content "Unicode"
    When I run "yg run 'ls -la && cat 测试.txt && cat émoji🚀.md'"
    Then the exit code should be 0
    And the output should contain "Chinese"
    And the output should contain "Unicode"
    And the output should contain "测试.txt"
    And the output should contain "émoji🚀.md"

  Scenario: Deeply nested directory sync
    Given I create nested directories 10 levels deep
    And I create file "a/b/c/d/e/f/g/h/i/j/deep.txt" with content "DEEP_CONTENT"
    When I run "yg run 'cat a/b/c/d/e/f/g/h/i/j/deep.txt'"
    Then the exit code should be 0
    And the output should contain "DEEP_CONTENT"

  Scenario: Large file with checksum validation
    Given I create a 100MB file named "medium.bin"
    And I compute checksum of "medium.bin" as "local_md5"
    When I run "yg run 'md5sum medium.bin'"
    Then the exit code should be 0
    And the output should contain the stored checksum

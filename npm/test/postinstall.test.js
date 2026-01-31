/**
 * Postinstall script tests.
 * Tests for the orchestration logic, environment variables, and error handling.
 */

const { describe, it, beforeEach, afterEach, mock } = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');
const os = require('os');

describe('postinstall module', () => {
  let testDir;
  let originalEnv;

  beforeEach(() => {
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'recall-postinstall-test-'));
    originalEnv = { ...process.env };
  });

  afterEach(() => {
    process.env = originalEnv;
    fs.rmSync(testDir, { recursive: true, force: true });
    // Clear module cache
    Object.keys(require.cache).forEach((key) => {
      if (key.includes('postinstall') || key.includes('lib/')) {
        delete require.cache[key];
      }
    });
  });

  describe('shouldSkipDownload', () => {
    it('returns true when RECALL_SKIP_DOWNLOAD=1', () => {
      process.env.RECALL_SKIP_DOWNLOAD = '1';

      const { shouldSkipDownload } = require('../lib/postinstall');
      assert.strictEqual(shouldSkipDownload(), true);
    });

    it('returns false when RECALL_SKIP_DOWNLOAD is not set', () => {
      delete process.env.RECALL_SKIP_DOWNLOAD;

      const { shouldSkipDownload } = require('../lib/postinstall');
      assert.strictEqual(shouldSkipDownload(), false);
    });

    it('returns false when RECALL_SKIP_DOWNLOAD is set to other value', () => {
      process.env.RECALL_SKIP_DOWNLOAD = '0';

      const { shouldSkipDownload } = require('../lib/postinstall');
      assert.strictEqual(shouldSkipDownload(), false);
    });
  });

  describe('getCustomBinaryPath', () => {
    it('returns custom path when RECALL_BINARY_PATH is set', () => {
      process.env.RECALL_BINARY_PATH = '/custom/path/to/recall';

      const { getCustomBinaryPath } = require('../lib/postinstall');
      assert.strictEqual(getCustomBinaryPath(), '/custom/path/to/recall');
    });

    it('returns null when RECALL_BINARY_PATH is not set', () => {
      delete process.env.RECALL_BINARY_PATH;

      const { getCustomBinaryPath } = require('../lib/postinstall');
      assert.strictEqual(getCustomBinaryPath(), null);
    });
  });

  describe('binaryExists', () => {
    it('returns true when binary exists', () => {
      const binaryPath = path.join(testDir, 'recall');
      fs.writeFileSync(binaryPath, 'binary content');

      const { binaryExists } = require('../lib/postinstall');
      assert.strictEqual(binaryExists(binaryPath), true);
    });

    it('returns false when binary does not exist', () => {
      const binaryPath = path.join(testDir, 'nonexistent');

      const { binaryExists } = require('../lib/postinstall');
      assert.strictEqual(binaryExists(binaryPath), false);
    });
  });

  describe('printManualInstructions', () => {
    it('prints manual installation instructions', () => {
      const logs = [];
      const originalError = console.error;
      console.error = (...args) => logs.push(args.join(' '));

      const { printManualInstructions } = require('../lib/postinstall');
      printManualInstructions(new Error('Test error'));

      console.error = originalError;

      const output = logs.join('\n');
      assert.ok(output.includes('Test error'), 'Should include error message');
      assert.ok(output.includes('Homebrew'), 'Should include Homebrew option');
      assert.ok(output.includes('github.com'), 'Should include GitHub releases');
      assert.ok(output.includes('RECALL_SKIP_DOWNLOAD'), 'Should mention skip env var');
      assert.ok(output.includes('RECALL_BINARY_PATH'), 'Should mention custom path env var');
    });
  });

  describe('formatProgress', () => {
    it('formats progress percentage', () => {
      const { formatProgress } = require('../lib/postinstall');

      assert.strictEqual(formatProgress(50, 100), 50);
      assert.strictEqual(formatProgress(25, 100), 25);
      assert.strictEqual(formatProgress(0, 100), 0);
      assert.strictEqual(formatProgress(100, 100), 100);
    });

    it('handles edge cases', () => {
      const { formatProgress } = require('../lib/postinstall');

      assert.strictEqual(formatProgress(0, 0), 0);
      assert.strictEqual(formatProgress(50, 0), 0);
    });
  });
});

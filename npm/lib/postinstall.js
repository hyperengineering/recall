/**
 * Postinstall helper functions.
 * Provides testable utilities for the postinstall script.
 */

'use strict';

const fs = require('fs');

/**
 * Check if download should be skipped via environment variable.
 * @returns {boolean}
 */
function shouldSkipDownload() {
  return process.env.RECALL_SKIP_DOWNLOAD === '1';
}

/**
 * Get custom binary path from environment variable.
 * @returns {string|null}
 */
function getCustomBinaryPath() {
  return process.env.RECALL_BINARY_PATH || null;
}

/**
 * Check if binary exists at the given path.
 * @param {string} binaryPath - Path to check
 * @returns {boolean}
 */
function binaryExists(binaryPath) {
  return fs.existsSync(binaryPath);
}

/**
 * Print manual installation instructions.
 * @param {Error} err - The error that occurred
 */
function printManualInstructions(err) {
  console.error('\n');
  console.error('Failed to install recall binary:');
  console.error(err.message);
  console.error('\n');
  console.error('Manual installation options:');
  console.error('  1. Homebrew (macOS/Linux): brew install hyperengineering/tap/recall');
  console.error('  2. Download from: https://github.com/hyperengineering/recall/releases');
  console.error('  3. Go install: go install github.com/hyperengineering/recall/cmd/recall@latest');
  console.error('\n');
  console.error('To skip this download, set RECALL_SKIP_DOWNLOAD=1');
  console.error('To use a custom binary, set RECALL_BINARY_PATH=/path/to/recall');
  console.error('\n');
}

/**
 * Format download progress as percentage.
 * @param {number} received - Bytes received
 * @param {number} total - Total bytes
 * @returns {number} - Percentage (0-100)
 */
function formatProgress(received, total) {
  if (total === 0) return 0;
  return Math.round((received / total) * 100);
}

module.exports = {
  shouldSkipDownload,
  getCustomBinaryPath,
  binaryExists,
  printManualInstructions,
  formatProgress
};

#!/usr/bin/env node

/**
 * Wrapper script for recall binary.
 * Executes the downloaded binary with all arguments.
 */

'use strict';

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

const SCRIPT_DIR = __dirname;
const VENDOR_DIR = path.join(SCRIPT_DIR, '..', 'vendor');

function getBinaryPath() {
  // Check for custom binary path
  if (process.env.RECALL_BINARY_PATH) {
    return process.env.RECALL_BINARY_PATH;
  }

  // Determine binary name based on platform
  const binaryName = process.platform === 'win32' ? 'recall.exe' : 'recall';
  const binaryPath = path.join(VENDOR_DIR, binaryName);

  return binaryPath;
}

function main() {
  const binaryPath = getBinaryPath();

  if (!fs.existsSync(binaryPath)) {
    console.error(`Error: recall binary not found at ${binaryPath}`);
    console.error('');
    console.error('Run one of the following to download the binary:');
    console.error('  npm rebuild @hyperengineering/recall');
    console.error('');
    console.error('Or set RECALL_BINARY_PATH to point to your recall installation');
    process.exit(1);
  }

  // Pass through all arguments to the binary
  const args = process.argv.slice(2);

  const child = spawn(binaryPath, args, {
    stdio: 'inherit',
    windowsHide: true
  });

  child.on('error', (err) => {
    console.error(`Error executing recall: ${err.message}`);
    process.exit(1);
  });

  child.on('exit', (code, signal) => {
    if (signal) {
      process.exit(1);
    }
    process.exit(code ?? 0);
  });
}

main();

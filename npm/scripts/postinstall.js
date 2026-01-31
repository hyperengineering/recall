#!/usr/bin/env node

/**
 * Postinstall script for @hyperengineering/recall.
 * Downloads and installs the recall binary for the current platform.
 */

'use strict';

const fs = require('fs');
const path = require('path');
const { getPlatform, getDownloadUrl, getBinaryName } = require('../lib/platform');
const { download } = require('../lib/download');
const { extract, setExecutable } = require('../lib/extract');
const {
  shouldSkipDownload,
  getCustomBinaryPath,
  binaryExists,
  printManualInstructions,
  formatProgress
} = require('../lib/postinstall');

const packageJson = require('../package.json');
const VERSION = packageJson.version;

const VENDOR_DIR = path.join(__dirname, '..', 'vendor');
const CACHE_DIR = path.join(__dirname, '..', '.cache');

async function main() {
  // Check for skip flag
  if (shouldSkipDownload()) {
    console.log('RECALL_SKIP_DOWNLOAD is set, skipping binary download.');
    return;
  }

  // Check for custom binary path
  const customPath = getCustomBinaryPath();
  if (customPath) {
    console.log(`Using custom binary path: ${customPath}`);
    return;
  }

  try {
    const platform = getPlatform();
    const binaryName = getBinaryName(platform);
    const binaryPath = path.join(VENDOR_DIR, binaryName);

    // Check if binary already exists
    if (binaryExists(binaryPath)) {
      console.log(`recall binary already exists at ${binaryPath}`);
      return;
    }

    console.log(`Installing recall v${VERSION} for ${platform.os}/${platform.arch}...`);

    // Create directories
    if (!fs.existsSync(VENDOR_DIR)) {
      fs.mkdirSync(VENDOR_DIR, { recursive: true });
    }
    if (!fs.existsSync(CACHE_DIR)) {
      fs.mkdirSync(CACHE_DIR, { recursive: true });
    }

    // Download archive
    const downloadUrl = getDownloadUrl(VERSION, platform);
    const archiveName = `recall_${VERSION}_${platform.os}_${platform.arch}.${platform.ext}`;
    const archivePath = path.join(CACHE_DIR, archiveName);

    console.log(`Downloading from ${downloadUrl}...`);
    await download(downloadUrl, archivePath, {
      onProgress: (received, total) => {
        const percent = formatProgress(received, total);
        process.stdout.write(`\rDownloading: ${percent}%`);
      }
    });
    console.log('\nDownload complete.');

    // Extract binary
    console.log('Extracting binary...');
    await extract(archivePath, VENDOR_DIR, platform);

    // Set executable permissions
    setExecutable(binaryPath);

    // Verify binary exists
    if (!binaryExists(binaryPath)) {
      throw new Error(`Binary not found after extraction: ${binaryPath}`);
    }

    // Clean up archive
    try {
      fs.unlinkSync(archivePath);
    } catch {
      // Ignore cleanup errors
    }

    console.log(`recall v${VERSION} installed successfully!`);
    console.log(`Binary location: ${binaryPath}`);

  } catch (err) {
    printManualInstructions(err);

    // Don't fail npm install - just warn
    process.exit(0);
  }
}

main();

/**
 * Archive extraction for tar.gz and zip files.
 */

'use strict';

const fs = require('fs');
const path = require('path');

/**
 * Extract archive to destination directory.
 * @param {string} archivePath - Path to archive file
 * @param {string} destDir - Destination directory
 * @param {{ ext: string, os?: string }} platform - Platform info with extension
 * @returns {Promise<void>}
 */
async function extract(archivePath, destDir, platform) {
  if (!fs.existsSync(destDir)) {
    fs.mkdirSync(destDir, { recursive: true });
  }

  if (platform.ext === 'zip') {
    await extractZip(archivePath, destDir);
  } else {
    await extractTarGz(archivePath, destDir);
  }
}

/**
 * Extract tar.gz archive, filtering for recall binary only.
 * @param {string} archivePath - Path to tar.gz file
 * @param {string} destDir - Destination directory
 * @returns {Promise<void>}
 */
async function extractTarGz(archivePath, destDir) {
  const tar = require('tar');

  await tar.extract({
    file: archivePath,
    cwd: destDir,
    filter: (entryPath) => {
      // Only extract the recall binary, skip README etc.
      const basename = path.basename(entryPath);
      return basename === 'recall' || basename === 'recall.exe';
    }
  });
}

/**
 * Extract zip archive, filtering for recall binary only.
 * @param {string} archivePath - Path to zip file
 * @param {string} destDir - Destination directory
 * @returns {Promise<void>}
 */
async function extractZip(archivePath, destDir) {
  const AdmZip = require('adm-zip');
  const zip = new AdmZip(archivePath);

  zip.getEntries().forEach((entry) => {
    const basename = path.basename(entry.entryName);
    if (basename === 'recall' || basename === 'recall.exe') {
      zip.extractEntryTo(entry, destDir, false, true);
    }
  });
}

/**
 * Set executable permissions on the binary (Unix only).
 * @param {string} binaryPath - Path to binary
 */
function setExecutable(binaryPath) {
  if (process.platform !== 'win32') {
    fs.chmodSync(binaryPath, 0o755);
  }
}

module.exports = { extract, setExecutable };

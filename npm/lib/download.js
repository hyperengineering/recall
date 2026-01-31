/**
 * Binary download with retry logic.
 */

'use strict';

const https = require('https');
const http = require('http');
const fs = require('fs');
const path = require('path');

const MAX_RETRIES = 3;
const RETRY_DELAY_MS = 1000;
const MAX_REDIRECTS = 10;
const REQUEST_TIMEOUT_MS = 30000;

/**
 * Download a file from URL to destination path.
 * @param {string} url - Download URL
 * @param {string} destPath - Destination file path
 * @param {object} options - Options
 * @param {number} [options.retries=3] - Number of retries
 * @param {function} [options.onProgress] - Progress callback (received, total)
 * @returns {Promise<void>}
 */
async function download(url, destPath, options = {}) {
  const { retries = MAX_RETRIES, onProgress } = options;

  for (let attempt = 1; attempt <= retries; attempt++) {
    try {
      await downloadOnce(url, destPath, onProgress);
      return;
    } catch (err) {
      if (attempt === retries) {
        throw err;
      }
      const delay = RETRY_DELAY_MS * Math.pow(2, attempt - 1);
      console.log(`Download failed (attempt ${attempt}/${retries}), retrying in ${delay}ms...`);
      await sleep(delay);
    }
  }
}

/**
 * Download a file once (no retries).
 * @param {string} url - Download URL
 * @param {string} destPath - Destination file path
 * @param {function} [onProgress] - Progress callback
 * @param {number} [redirectCount=0] - Current redirect count
 * @returns {Promise<void>}
 */
function downloadOnce(url, destPath, onProgress, redirectCount = 0) {
  return new Promise((resolve, reject) => {
    const protocol = url.startsWith('https') ? https : http;

    const request = protocol.get(url, {
      headers: { 'User-Agent': 'npm-@hyperengineering/recall' },
      timeout: REQUEST_TIMEOUT_MS
    }, (response) => {
      // Handle redirects with limit
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        if (redirectCount >= MAX_REDIRECTS) {
          reject(new Error(`Too many redirects (max ${MAX_REDIRECTS})`));
          return;
        }
        downloadOnce(response.headers.location, destPath, onProgress, redirectCount + 1)
          .then(resolve)
          .catch(reject);
        return;
      }

      if (response.statusCode !== 200) {
        reject(new Error(`Download failed: HTTP ${response.statusCode}`));
        return;
      }

      const totalBytes = parseInt(response.headers['content-length'], 10) || 0;
      let receivedBytes = 0;

      const dir = path.dirname(destPath);
      if (!fs.existsSync(dir)) {
        fs.mkdirSync(dir, { recursive: true });
      }

      const fileStream = fs.createWriteStream(destPath);

      response.on('data', (chunk) => {
        receivedBytes += chunk.length;
        if (onProgress && totalBytes > 0) {
          onProgress(receivedBytes, totalBytes);
        }
      });

      response.pipe(fileStream);

      fileStream.on('finish', () => {
        fileStream.close();
        resolve();
      });

      fileStream.on('error', (err) => {
        fs.unlink(destPath, () => {}); // Clean up partial file
        reject(err);
      });
    });

    request.on('error', (err) => {
      // Clean up partial file if it exists
      if (fs.existsSync(destPath)) {
        fs.unlinkSync(destPath);
      }
      reject(err);
    });

    request.on('timeout', () => {
      request.destroy();
      reject(new Error('Download timed out'));
    });
  });
}

/**
 * Sleep for a given number of milliseconds.
 * @param {number} ms - Milliseconds to sleep
 * @returns {Promise<void>}
 */
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

module.exports = { download };

/**
 * Platform detection for binary downloads.
 * Maps Node.js platform/arch to GoReleaser naming conventions.
 */

'use strict';

const PLATFORM_MAP = {
  darwin: 'darwin',
  linux: 'linux',
  win32: 'windows'
};

const ARCH_MAP = {
  x64: 'amd64',
  arm64: 'arm64'
};

/**
 * Get the current platform info for binary download.
 * @returns {{ os: string, arch: string, ext: string }}
 */
function getPlatform() {
  const platform = process.platform;
  const arch = process.arch;

  const os = PLATFORM_MAP[platform];
  if (!os) {
    throw new Error(`Unsupported platform: ${platform}. Supported: darwin, linux, win32`);
  }

  const goArch = ARCH_MAP[arch];
  if (!goArch) {
    throw new Error(`Unsupported architecture: ${arch}. Supported: x64, arm64`);
  }

  // Windows arm64 is not supported by GoReleaser config
  if (os === 'windows' && goArch === 'arm64') {
    throw new Error('Windows ARM64 is not currently supported. Please use x64.');
  }

  const ext = os === 'windows' ? 'zip' : 'tar.gz';

  return { os, arch: goArch, ext };
}

/**
 * Construct the GitHub release download URL.
 * @param {string} version - Package version (e.g., "1.2.3")
 * @param {{ os: string, arch: string, ext: string }} platform
 * @returns {string}
 */
function getDownloadUrl(version, platform) {
  const { os, arch, ext } = platform;
  const filename = `recall_${version}_${os}_${arch}.${ext}`;
  return `https://github.com/hyperengineering/recall/releases/download/v${version}/${filename}`;
}

/**
 * Get the binary filename for the current platform.
 * @param {{ os: string }} platform
 * @returns {string}
 */
function getBinaryName(platform) {
  return platform.os === 'windows' ? 'recall.exe' : 'recall';
}

module.exports = { getPlatform, getDownloadUrl, getBinaryName };

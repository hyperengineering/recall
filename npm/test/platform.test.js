/**
 * Platform detection tests.
 * Tests for OS/arch mapping to GoReleaser naming conventions.
 */

const { describe, it, mock } = require('node:test');
const assert = require('node:assert');

describe('platform detection', () => {
  describe('getPlatform', () => {
    it('detects macOS x64 correctly', (t) => {
      const platform = require('../lib/platform');

      // Mock process.platform and process.arch
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'darwin', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'x64', configurable: true });

      try {
        // Clear module cache to pick up new mocks
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        const result = getPlatform();
        assert.deepStrictEqual(result, { os: 'darwin', arch: 'amd64', ext: 'tar.gz' });
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });

    it('detects macOS arm64 correctly', () => {
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'darwin', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'arm64', configurable: true });

      try {
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        const result = getPlatform();
        assert.deepStrictEqual(result, { os: 'darwin', arch: 'arm64', ext: 'tar.gz' });
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });

    it('detects Linux x64 correctly', () => {
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'linux', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'x64', configurable: true });

      try {
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        const result = getPlatform();
        assert.deepStrictEqual(result, { os: 'linux', arch: 'amd64', ext: 'tar.gz' });
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });

    it('detects Linux arm64 correctly', () => {
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'linux', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'arm64', configurable: true });

      try {
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        const result = getPlatform();
        assert.deepStrictEqual(result, { os: 'linux', arch: 'arm64', ext: 'tar.gz' });
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });

    it('detects Windows x64 correctly with zip extension', () => {
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'win32', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'x64', configurable: true });

      try {
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        const result = getPlatform();
        assert.deepStrictEqual(result, { os: 'windows', arch: 'amd64', ext: 'zip' });
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });

    it('throws error for Windows arm64 (not supported)', () => {
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'win32', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'arm64', configurable: true });

      try {
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        assert.throws(
          () => getPlatform(),
          /Windows ARM64 is not currently supported/
        );
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });

    it('throws error for unsupported platform', () => {
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'freebsd', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'x64', configurable: true });

      try {
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        assert.throws(
          () => getPlatform(),
          /Unsupported platform: freebsd/
        );
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });

    it('throws error for unsupported architecture', () => {
      const originalPlatform = process.platform;
      const originalArch = process.arch;

      Object.defineProperty(process, 'platform', { value: 'linux', configurable: true });
      Object.defineProperty(process, 'arch', { value: 'ia32', configurable: true });

      try {
        delete require.cache[require.resolve('../lib/platform')];
        const { getPlatform } = require('../lib/platform');

        assert.throws(
          () => getPlatform(),
          /Unsupported architecture: ia32/
        );
      } finally {
        Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
        Object.defineProperty(process, 'arch', { value: originalArch, configurable: true });
      }
    });
  });

  describe('getDownloadUrl', () => {
    it('constructs correct URL for darwin amd64', () => {
      const { getDownloadUrl } = require('../lib/platform');

      const url = getDownloadUrl('1.2.3', { os: 'darwin', arch: 'amd64', ext: 'tar.gz' });

      assert.strictEqual(
        url,
        'https://github.com/hyperengineering/recall/releases/download/v1.2.3/recall_1.2.3_darwin_amd64.tar.gz'
      );
    });

    it('constructs correct URL for linux arm64', () => {
      const { getDownloadUrl } = require('../lib/platform');

      const url = getDownloadUrl('2.0.0', { os: 'linux', arch: 'arm64', ext: 'tar.gz' });

      assert.strictEqual(
        url,
        'https://github.com/hyperengineering/recall/releases/download/v2.0.0/recall_2.0.0_linux_arm64.tar.gz'
      );
    });

    it('constructs correct URL for windows amd64 with zip extension', () => {
      const { getDownloadUrl } = require('../lib/platform');

      const url = getDownloadUrl('1.0.0', { os: 'windows', arch: 'amd64', ext: 'zip' });

      assert.strictEqual(
        url,
        'https://github.com/hyperengineering/recall/releases/download/v1.0.0/recall_1.0.0_windows_amd64.zip'
      );
    });
  });

  describe('getBinaryName', () => {
    it('returns recall for Unix platforms', () => {
      const { getBinaryName } = require('../lib/platform');

      assert.strictEqual(getBinaryName({ os: 'darwin' }), 'recall');
      assert.strictEqual(getBinaryName({ os: 'linux' }), 'recall');
    });

    it('returns recall.exe for Windows', () => {
      const { getBinaryName } = require('../lib/platform');

      assert.strictEqual(getBinaryName({ os: 'windows' }), 'recall.exe');
    });
  });
});

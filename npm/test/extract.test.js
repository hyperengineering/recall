/**
 * Extract module tests.
 * Tests for tar.gz and zip archive extraction.
 */

const { describe, it, beforeEach, afterEach } = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { execSync } = require('child_process');

describe('extract module', () => {
  let testDir;
  let archiveDir;
  let extractDir;

  beforeEach(() => {
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'recall-extract-test-'));
    archiveDir = path.join(testDir, 'archives');
    extractDir = path.join(testDir, 'extracted');
    fs.mkdirSync(archiveDir);
    fs.mkdirSync(extractDir);
  });

  afterEach(() => {
    fs.rmSync(testDir, { recursive: true, force: true });
  });

  function createTestTarGz(archivePath, files) {
    // Create a temp directory with the files
    const contentDir = path.join(testDir, 'tar-content');
    fs.mkdirSync(contentDir, { recursive: true });

    for (const [name, content] of Object.entries(files)) {
      fs.writeFileSync(path.join(contentDir, name), content);
    }

    // Create tar.gz using system tar
    execSync(`tar -czf "${archivePath}" -C "${contentDir}" .`, { stdio: 'pipe' });
  }

  function createTestZip(archivePath, files) {
    const AdmZip = require('adm-zip');
    const zip = new AdmZip();

    for (const [name, content] of Object.entries(files)) {
      zip.addFile(name, Buffer.from(content));
    }

    zip.writeZip(archivePath);
  }

  describe('extract tar.gz', () => {
    it('extracts binary from tar.gz archive', async () => {
      const archivePath = path.join(archiveDir, 'test.tar.gz');
      const binaryContent = '#!/bin/sh\necho "Hello from recall"';

      createTestTarGz(archivePath, {
        'recall': binaryContent,
        'README.md': 'This is readme'
      });

      const { extract } = require('../lib/extract');
      await extract(archivePath, extractDir, { ext: 'tar.gz' });

      const binaryPath = path.join(extractDir, 'recall');
      assert.ok(fs.existsSync(binaryPath), 'Binary should be extracted');
      assert.strictEqual(fs.readFileSync(binaryPath, 'utf8'), binaryContent);
    });

    it('only extracts recall binary from tar.gz, ignores other files', async () => {
      const archivePath = path.join(archiveDir, 'test.tar.gz');

      createTestTarGz(archivePath, {
        'recall': 'binary content',
        'README.md': 'readme content',
        'LICENSE': 'license content'
      });

      const { extract } = require('../lib/extract');
      await extract(archivePath, extractDir, { ext: 'tar.gz' });

      assert.ok(fs.existsSync(path.join(extractDir, 'recall')), 'Binary should be extracted');
      assert.ok(!fs.existsSync(path.join(extractDir, 'README.md')), 'README should not be extracted');
      assert.ok(!fs.existsSync(path.join(extractDir, 'LICENSE')), 'LICENSE should not be extracted');
    });
  });

  describe('extract zip', () => {
    it('extracts binary from zip archive', async () => {
      const archivePath = path.join(archiveDir, 'test.zip');
      const binaryContent = 'Windows executable content';

      createTestZip(archivePath, {
        'recall.exe': binaryContent,
        'README.md': 'This is readme'
      });

      const { extract } = require('../lib/extract');
      await extract(archivePath, extractDir, { ext: 'zip', os: 'windows' });

      const binaryPath = path.join(extractDir, 'recall.exe');
      assert.ok(fs.existsSync(binaryPath), 'Binary should be extracted');
      assert.strictEqual(fs.readFileSync(binaryPath, 'utf8'), binaryContent);
    });

    it('only extracts recall binary from zip, ignores other files', async () => {
      const archivePath = path.join(archiveDir, 'test.zip');

      createTestZip(archivePath, {
        'recall.exe': 'binary content',
        'README.md': 'readme content',
        'LICENSE': 'license content'
      });

      const { extract } = require('../lib/extract');
      await extract(archivePath, extractDir, { ext: 'zip', os: 'windows' });

      assert.ok(fs.existsSync(path.join(extractDir, 'recall.exe')), 'Binary should be extracted');
      assert.ok(!fs.existsSync(path.join(extractDir, 'README.md')), 'README should not be extracted');
      assert.ok(!fs.existsSync(path.join(extractDir, 'LICENSE')), 'LICENSE should not be extracted');
    });
  });

  describe('setExecutable', () => {
    it('sets executable permissions on Unix', async function() {
      // Skip on Windows
      if (process.platform === 'win32') {
        this.skip();
        return;
      }

      const binaryPath = path.join(extractDir, 'recall');
      fs.writeFileSync(binaryPath, '#!/bin/sh\necho "test"');

      const { setExecutable } = require('../lib/extract');
      setExecutable(binaryPath);

      const stats = fs.statSync(binaryPath);
      const mode = stats.mode & 0o777;

      // Check that executable bits are set (at least owner execute)
      assert.ok((mode & 0o100) !== 0, 'Owner execute bit should be set');
    });

    it('does nothing on Windows', () => {
      const { setExecutable } = require('../lib/extract');

      // Should not throw even if file doesn't exist on Windows
      // (since it's a no-op on Windows)
      if (process.platform === 'win32') {
        assert.doesNotThrow(() => setExecutable('/nonexistent/file'));
      }
    });
  });

  describe('extract creates destination directory', () => {
    it('creates destination directory if it does not exist', async () => {
      const archivePath = path.join(archiveDir, 'test.tar.gz');
      const nestedExtractDir = path.join(testDir, 'nested', 'deep', 'extract');

      createTestTarGz(archivePath, {
        'recall': 'binary content'
      });

      const { extract } = require('../lib/extract');
      await extract(archivePath, nestedExtractDir, { ext: 'tar.gz' });

      assert.ok(fs.existsSync(path.join(nestedExtractDir, 'recall')), 'Binary should be extracted to nested dir');
    });
  });
});

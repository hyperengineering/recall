/**
 * Download module tests.
 * Tests for HTTP download with retry logic and redirect handling.
 */

const { describe, it, beforeEach, afterEach, mock } = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');
const os = require('os');
const http = require('http');

describe('download module', () => {
  let testDir;
  let server;
  let serverPort;

  beforeEach(() => {
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), 'recall-download-test-'));
  });

  afterEach(async () => {
    if (server) {
      await new Promise((resolve) => server.close(resolve));
      server = null;
    }
    fs.rmSync(testDir, { recursive: true, force: true });
  });

  function createTestServer(handler) {
    return new Promise((resolve) => {
      server = http.createServer(handler);
      server.listen(0, '127.0.0.1', () => {
        serverPort = server.address().port;
        resolve(serverPort);
      });
    });
  }

  describe('download', () => {
    it('downloads file successfully', async () => {
      const testContent = 'Hello, this is test content!';

      await createTestServer((req, res) => {
        res.writeHead(200, {
          'Content-Type': 'application/octet-stream',
          'Content-Length': Buffer.byteLength(testContent)
        });
        res.end(testContent);
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'test-file');

      await download(`http://127.0.0.1:${serverPort}/test`, destPath, { retries: 1 });

      assert.ok(fs.existsSync(destPath), 'Downloaded file should exist');
      assert.strictEqual(fs.readFileSync(destPath, 'utf8'), testContent);
    });

    it('follows HTTP redirects', async () => {
      const testContent = 'Redirected content';
      let requestCount = 0;

      await createTestServer((req, res) => {
        requestCount++;
        if (req.url === '/redirect') {
          res.writeHead(302, { Location: `http://127.0.0.1:${serverPort}/final` });
          res.end();
        } else if (req.url === '/final') {
          res.writeHead(200, {
            'Content-Type': 'application/octet-stream',
            'Content-Length': Buffer.byteLength(testContent)
          });
          res.end(testContent);
        }
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'redirected-file');

      await download(`http://127.0.0.1:${serverPort}/redirect`, destPath, { retries: 1 });

      assert.ok(fs.existsSync(destPath), 'Downloaded file should exist');
      assert.strictEqual(fs.readFileSync(destPath, 'utf8'), testContent);
      assert.strictEqual(requestCount, 2, 'Should have made 2 requests (redirect + final)');
    });

    it('retries on failure with exponential backoff', async () => {
      let requestCount = 0;
      const testContent = 'Success after retry';

      await createTestServer((req, res) => {
        requestCount++;
        if (requestCount < 3) {
          res.destroy(); // Simulate network error
        } else {
          res.writeHead(200, {
            'Content-Type': 'application/octet-stream',
            'Content-Length': Buffer.byteLength(testContent)
          });
          res.end(testContent);
        }
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'retry-file');

      await download(`http://127.0.0.1:${serverPort}/test`, destPath, { retries: 3 });

      assert.ok(fs.existsSync(destPath), 'Downloaded file should exist');
      assert.strictEqual(fs.readFileSync(destPath, 'utf8'), testContent);
      assert.strictEqual(requestCount, 3, 'Should have made 3 requests');
    });

    it('throws error after max retries exceeded', async () => {
      await createTestServer((req, res) => {
        res.destroy(); // Always fail
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'failed-file');

      await assert.rejects(
        download(`http://127.0.0.1:${serverPort}/test`, destPath, { retries: 2 }),
        /socket hang up|ECONNRESET/
      );

      assert.ok(!fs.existsSync(destPath), 'Failed download should not create file');
    });

    it('throws error on HTTP 404', async () => {
      await createTestServer((req, res) => {
        res.writeHead(404, { 'Content-Type': 'text/plain' });
        res.end('Not Found');
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'notfound-file');

      await assert.rejects(
        download(`http://127.0.0.1:${serverPort}/test`, destPath, { retries: 1 }),
        /Download failed: HTTP 404/
      );
    });

    it('throws error after too many redirects', async () => {
      await createTestServer((req, res) => {
        // Always redirect to self - infinite loop
        res.writeHead(302, { Location: `http://127.0.0.1:${serverPort}/redirect` });
        res.end();
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'redirect-loop-file');

      await assert.rejects(
        download(`http://127.0.0.1:${serverPort}/redirect`, destPath, { retries: 1 }),
        /Too many redirects/
      );
    });

    it('calls progress callback during download', async () => {
      const testContent = 'Progress test content that is reasonably long';
      const progressCalls = [];

      await createTestServer((req, res) => {
        res.writeHead(200, {
          'Content-Type': 'application/octet-stream',
          'Content-Length': Buffer.byteLength(testContent)
        });
        res.end(testContent);
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'progress-file');

      await download(`http://127.0.0.1:${serverPort}/test`, destPath, {
        retries: 1,
        onProgress: (received, total) => {
          progressCalls.push({ received, total });
        }
      });

      assert.ok(progressCalls.length > 0, 'Progress callback should have been called');
      const lastCall = progressCalls[progressCalls.length - 1];
      assert.strictEqual(lastCall.received, Buffer.byteLength(testContent));
      assert.strictEqual(lastCall.total, Buffer.byteLength(testContent));
    });

    it('creates destination directory if it does not exist', async () => {
      const testContent = 'Nested directory content';

      await createTestServer((req, res) => {
        res.writeHead(200, {
          'Content-Type': 'application/octet-stream',
          'Content-Length': Buffer.byteLength(testContent)
        });
        res.end(testContent);
      });

      const { download } = require('../lib/download');
      const nestedDir = path.join(testDir, 'nested', 'deep', 'dir');
      const destPath = path.join(nestedDir, 'file');

      await download(`http://127.0.0.1:${serverPort}/test`, destPath, { retries: 1 });

      assert.ok(fs.existsSync(destPath), 'Downloaded file should exist in nested directory');
      assert.strictEqual(fs.readFileSync(destPath, 'utf8'), testContent);
    });

    it('includes User-Agent header in requests', async () => {
      let receivedUserAgent = null;

      await createTestServer((req, res) => {
        receivedUserAgent = req.headers['user-agent'];
        res.writeHead(200, { 'Content-Type': 'application/octet-stream' });
        res.end('test');
      });

      const { download } = require('../lib/download');
      const destPath = path.join(testDir, 'ua-file');

      await download(`http://127.0.0.1:${serverPort}/test`, destPath, { retries: 1 });

      assert.ok(receivedUserAgent, 'User-Agent header should be present');
      assert.ok(
        receivedUserAgent.includes('@hyperengineering/recall'),
        'User-Agent should identify the package'
      );
    });
  });
});

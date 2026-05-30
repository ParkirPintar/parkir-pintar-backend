const http = require('http');
const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

const PORT = 3000;
const STATIC_DIR = __dirname;
const PROJECT_ROOT = path.resolve(__dirname, '..', '..');

const MIME_TYPES = {
    '.html': 'text/html',
    '.js': 'application/javascript',
    '.css': 'text/css',
    '.json': 'application/json',
    '.png': 'image/png',
    '.svg': 'image/svg+xml',
    '.xml': 'application/xml',
    '.drawio': 'application/xml',
};

function serveStatic(req, res) {
    let filePath = path.join(STATIC_DIR, req.url === '/' ? 'index.html' : req.url);
    filePath = decodeURIComponent(filePath);

    if (!filePath.startsWith(STATIC_DIR)) {
        res.writeHead(403);
        res.end('Forbidden');
        return;
    }

    const ext = path.extname(filePath);
    const contentType = MIME_TYPES[ext] || 'application/octet-stream';

    fs.readFile(filePath, (err, data) => {
        if (err) {
            res.writeHead(404);
            res.end('Not found');
            return;
        }
        res.writeHead(200, { 'Content-Type': contentType });
        res.end(data);
    });
}

function runNewman(req, res) {
    const url = new URL(req.url, `http://localhost:${PORT}`);
    const folder = url.searchParams.get('folder') || '';
    const baseUrl = url.searchParams.get('baseUrl') || 'https://parkir-pintar.pondongopi.biz.id';

    // SSE headers
    res.writeHead(200, {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        'Connection': 'keep-alive',
        'Access-Control-Allow-Origin': '*',
    });

    const collectionPath = path.join(PROJECT_ROOT, 'sre/e2e/parkir-pintar.postman_collection.json');
    const envPath = path.join(PROJECT_ROOT, 'sre/e2e/parkir-pintar.postman_environment.json');

    // Check if files exist
    if (!fs.existsSync(collectionPath)) {
        res.write(`data: ${JSON.stringify({ type: 'error', text: `Collection not found: ${collectionPath}` })}\n\n`);
        res.end();
        return;
    }

    const args = [
        'run', collectionPath,
        '-e', envPath,
        '--env-var', `base_url=${baseUrl}`,
        '--delay-request', '2000',
        '--color', 'off',
    ];

    if (folder) {
        args.push('--folder', folder);
    }

    res.write(`data: ${JSON.stringify({ type: 'info', text: `$ newman ${args.join(' ')}` })}\n\n`);
    res.write(`data: ${JSON.stringify({ type: 'info', text: '─'.repeat(60) })}\n\n`);

    const proc = spawn('newman', args, { cwd: PROJECT_ROOT });

    proc.stdout.on('data', (data) => {
        const lines = data.toString().split('\n');
        for (const line of lines) {
            if (line.trim()) {
                res.write(`data: ${JSON.stringify({ type: 'stdout', text: line })}\n\n`);
            }
        }
    });

    proc.stderr.on('data', (data) => {
        const lines = data.toString().split('\n');
        for (const line of lines) {
            if (line.trim()) {
                res.write(`data: ${JSON.stringify({ type: 'stderr', text: line })}\n\n`);
            }
        }
    });

    proc.on('close', (code) => {
        res.write(`data: ${JSON.stringify({ type: 'done', code: code, text: code === 0 ? '✅ All tests passed!' : `❌ Exited with code ${code}` })}\n\n`);
        res.end();
    });

    proc.on('error', (err) => {
        res.write(`data: ${JSON.stringify({ type: 'error', text: `Failed to start newman: ${err.message}. Install with: npm install -g newman` })}\n\n`);
        res.end();
    });

    // Handle client disconnect
    req.on('close', () => {
        proc.kill('SIGTERM');
    });
}

const server = http.createServer((req, res) => {
    if (req.url.startsWith('/api/newman')) {
        runNewman(req, res);
    } else {
        serveStatic(req, res);
    }
});

server.listen(PORT, () => {
    console.log(`
┌─────────────────────────────────────────────────┐
│  🅿️  ParkirPintar Diagram Viewer                 │
│                                                 │
│  → http://localhost:${PORT}                        │
│                                                 │
│  Features:                                      │
│  • Drawio diagrams (HLD, LLD, Business Flow)    │
│  • Mermaid (ERD, Sequence, Component)           │
│  • Newman E2E runner (live output)              │
│                                                 │
│  Press Ctrl+C to stop                           │
└─────────────────────────────────────────────────┘
`);
});

const http = require('http');
const url = require('url');
const querystring = require('querystring');

const port = process.env.SERVER_PORT || 8080;
const serviceName = 'nodejs';

const server = http.createServer(async (req, res) => {
  const parsedUrl = url.parse(req.url, true);
  const pathname = parsedUrl.pathname;
  const method = req.method;

  res.setHeader('Content-Type', 'application/json');

  if (pathname === '/health' && method === 'GET') {
    res.writeHead(200);
    res.end(JSON.stringify({ status: 'ok' }));
    return;
  }

  if (pathname === '/chain' && method === 'POST') {
    handleChain(req, res);
    return;
  }

  res.writeHead(404);
  res.end(JSON.stringify({ error: 'Not found' }));
});

async function handleChain(req, res) {
  try {
    // Read request body
    const body = await readBody(req);
    let data;
    try {
      data = JSON.parse(body);
    } catch (e) {
      res.writeHead(400);
      res.end(JSON.stringify({
        service: serviceName,
        status: 400,
        error: `Failed to parse JSON: ${e.message}`
      }));
      return;
    }

    const targets = data.targets || [];
    console.error(`Received chain request with ${targets.length} targets`);

    // If no targets, return success (end of chain)
    if (targets.length === 0) {
      res.writeHead(200);
      res.end(JSON.stringify({
        service: serviceName,
        status: 200,
        targets: targets,
        result: 'Chain completed'
      }));
      return;
    }

    // Forward to next target with remaining targets
    const nextTarget = targets[0];
    const remainingTargets = targets.slice(1);

    console.error(`Forwarding to ${nextTarget} with ${remainingTargets.length} remaining targets`);

    const nextReq = {
      targets: remainingTargets
    };

    try {
      const chainUrl = `http://${nextTarget}/chain`;
      const response = await makeRequest(chainUrl, nextReq);

      res.writeHead(response.statusCode);
      res.end(JSON.stringify(response.body));
    } catch (error) {
      res.writeHead(502);
      res.end(JSON.stringify({
        service: serviceName,
        status: 502,
        targets: targets,
        error: `Failed to call ${nextTarget}: ${error.message}`
      }));
    }

  } catch (error) {
    res.writeHead(400);
    res.end(JSON.stringify({
      service: serviceName,
      status: 400,
      error: `Error processing request: ${error.message}`
    }));
  }
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let body = '';
    req.on('data', chunk => {
      body += chunk.toString();
    });
    req.on('end', () => {
      resolve(body);
    });
    req.on('error', reject);
  });
}

function makeRequest(targetUrl, data) {
  return new Promise((resolve, reject) => {
    const parsedUrl = url.parse(targetUrl);
    const postData = JSON.stringify(data);

    const options = {
      hostname: parsedUrl.hostname,
      port: parsedUrl.port || 80,
      path: parsedUrl.path || '/',
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(postData)
      },
      timeout: 10000
    };

    const req = http.request(options, (res) => {
      let body = '';
      res.on('data', chunk => {
        body += chunk.toString();
      });
      res.on('end', () => {
        try {
          const parsed = JSON.parse(body);
          resolve({
            statusCode: res.statusCode,
            body: parsed
          });
        } catch (e) {
          resolve({
            statusCode: res.statusCode,
            body: body
          });
        }
      });
    });

    req.on('error', reject);
    req.on('timeout', () => {
      req.destroy();
      reject(new Error('Request timeout'));
    });

    req.write(postData);
    req.end();
  });
}

server.listen(port, () => {
  console.error(`Starting server on port ${port}`);
});

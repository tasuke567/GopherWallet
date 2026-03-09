import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const transferSuccess = new Counter('transfer_success');
const transferFailed = new Counter('transfer_failed');
const transferRate = new Rate('transfer_success_rate');
const transferDuration = new Trend('transfer_duration', true);

// Test configuration — ramps up traffic to simulate real load
export const options = {
  stages: [
    { duration: '10s', target: 10 },   // warm up
    { duration: '30s', target: 50 },   // ramp to 50 users
    { duration: '30s', target: 100 },  // peak: 100 concurrent users
    { duration: '20s', target: 50 },   // scale down
    { duration: '10s', target: 0 },    // cool down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],  // 95% < 500ms, 99% < 1s
    transfer_success_rate: ['rate>0.90'],              // 90%+ success
    http_req_failed: ['rate<0.10'],                    // <10% HTTP errors
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// --- Setup: create test accounts ---
export function setup() {
  console.log('Creating test accounts...');

  const accounts = [];
  for (let i = 0; i < 20; i++) {
    const res = http.post(`${BASE_URL}/api/v1/accounts`, JSON.stringify({
      user_id: `loadtest-user-${i}`,
      balance: 10000000, // 100,000.00 THB
      currency: 'THB',
    }), { headers: { 'Content-Type': 'application/json' } });

    if (res.status === 201) {
      accounts.push(JSON.parse(res.body));
    }
  }

  console.log(`Created ${accounts.length} accounts`);
  return { accounts };
}

// --- Main test: transfer between random accounts ---
export default function (data) {
  const accounts = data.accounts;
  if (accounts.length < 2) {
    console.error('Not enough accounts for testing');
    return;
  }

  // Pick two different random accounts
  const fromIdx = Math.floor(Math.random() * accounts.length);
  let toIdx = Math.floor(Math.random() * accounts.length);
  while (toIdx === fromIdx) {
    toIdx = Math.floor(Math.random() * accounts.length);
  }

  const payload = JSON.stringify({
    from_account_id: accounts[fromIdx].id,
    to_account_id: accounts[toIdx].id,
    amount: Math.floor(Math.random() * 1000) + 100, // 1.00 - 11.00 THB
    idempotency_key: `k6-${__VU}-${__ITER}-${Date.now()}`,
  });

  const start = Date.now();
  const res = http.post(`${BASE_URL}/api/v1/transfers`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': `k6-${__VU}-${__ITER}-${Date.now()}`,
    },
  });
  const duration = Date.now() - start;

  transferDuration.add(duration);

  const success = check(res, {
    'status is 201': (r) => r.status === 201,
    'has transaction id': (r) => {
      try { return JSON.parse(r.body).id > 0; } catch { return false; }
    },
    'response time < 500ms': (r) => r.timings.duration < 500,
  });

  if (success) {
    transferSuccess.add(1);
    transferRate.add(1);
  } else {
    transferFailed.add(1);
    transferRate.add(0);
  }

  sleep(0.1); // 100ms think time between requests
}

// --- Teardown ---
export function teardown(data) {
  console.log('Load test completed!');
}

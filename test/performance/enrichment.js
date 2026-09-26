import http from 'k6/http';
import { check, sleep } from 'k6';
import { BASE_URL, login, authHeaders, perfEmail } from './common.js';

// Enrichment fans out to an external API; thresholds protect against
// regressions without expecting the upstream to be fast.
export const options = {
  vus: 5,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<2000'],
  },
};

export function setup() {
  return { token: login(perfEmail(0), 'password123') };
}

export default function (data) {
  const res = http.post(`${BASE_URL}/api/jobs/enrichment`,
    JSON.stringify({ count: 2 }), authHeaders(data.token));
  check(res, { 'enqueue ok': (r) => r.status === 202 });
  sleep(1);
}

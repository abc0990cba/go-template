import http from 'k6/http';
import { check, sleep } from 'k6';
import { BASE_URL, login, authHeaders, perfEmail } from './common.js';

// MODE=cursor|keyset|offset selects the pagination mode under load.
const MODE = __ENV.MODE || 'offset';
const P95 = { offset: 500, cursor: 300, keyset: 250 }[MODE] || 500;

export const options = {
  vus: 50,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    [`http_req_duration{name:list users}`]: [`p(95)<${P95}`],
  },
};

export function setup() {
  return { token: login(perfEmail(0), 'password123') };
}

export default function (data) {
  const after = MODE === 'offset' ? '' : `&after=perf-${(__VU * 100) % 90000}`;
  const res = http.get(`${BASE_URL}/api/users?mode=${MODE}&limit=100&after=${after}`,
    authHeaders(data.token));
  check(res, { 'list ok': (r) => r.status === 200 });
  sleep(0.2);
}

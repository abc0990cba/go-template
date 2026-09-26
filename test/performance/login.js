import http from 'k6/http';
import { check } from 'k6';
import { BASE_URL, perfEmail } from './common.js';

export const options = {
  vus: 50,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<300'],
  },
};

export default function () {
  const res = http.post(`${BASE_URL}/api/auth/login`,
    JSON.stringify({ email: perfEmail(__VU - 1), password: 'password123' }),
    { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'login ok': (r) => r.status === 200 });
}

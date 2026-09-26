import http from 'k6/http';
import { check } from 'k6';
import { BASE_URL } from './common.js';

export const options = {
  vus: 20,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

export default function () {
  const res = http.get(`${BASE_URL}/api/analytics/summary`);
  check(res, { 'summary ok': (r) => r.status === 200 });
}

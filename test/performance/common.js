import http from 'k6/http';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// login exchanges email/password for a bearer token; k6 checks fail the
// scenario when auth breaks.
export function login(email, password) {
  const res = http.post(`${BASE_URL}/api/auth/login`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' } });
  const body = res.json();
  if (!body.token) {
    throw new Error(`login failed: ${res.status} ${res.body}`);
  }
  return body.token;
}

export function authHeaders(token) {
  return {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
  };
}

export const perfEmail = (i) => `perf-${i}@example.com`;

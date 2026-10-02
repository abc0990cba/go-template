import { http, HttpResponse } from "msw";
import { problem } from "../fixtures";


export const auth = [
http.post("/api/auth/login", () =>
    problem(401, "invalid credentials", "email or password is wrong"),
  ),
http.post("/api/auth/mfa/challenge", () =>
    problem(401, "invalid code", "the code is wrong or expired"),
  ),
http.post("/api/auth/logout", () => new HttpResponse(null, { status: 204 })),
http.post("/api/auth/forgot-password", () => new HttpResponse(null, { status: 204 })),
  // The reset request always succeeds from the browser's point of view.
  http.post("/api/auth/forgot-password", () => new HttpResponse(null, { status: 204 })),

];

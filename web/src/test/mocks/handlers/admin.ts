import { http, HttpResponse } from "msw";
import { page } from "../fixtures";


export const admin = [
http.get("/api/roles", () => HttpResponse.json(page([]))),
http.get("/api/jobs", () => HttpResponse.json(page([]))),
  http.get("/api/roles", () => HttpResponse.json(page([]))),
];

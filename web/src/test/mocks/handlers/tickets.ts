import { http, HttpResponse } from "msw";
import { page, ticketFixture } from "../fixtures";

export const tickets = [
  http.get("/api/tickets", () => HttpResponse.json(page([]))),
  http.get("/api/tickets/counts", () =>
    HttpResponse.json({
      draft: 0,
      in_review: 0,
      approved: 0,
      completed: 0,
      rejected: 0,
      withdrawn: 0,
    }),
  ),
  // The approvals path serves both the inbox and the decided history;
  // the state query parameter picks which side answers.
  http.get("/api/approvals", ({ request }) => {
    const state = new URL(request.url).searchParams.get("state");
    return HttpResponse.json(state === "decided" ? page([]) : page([]));
  }),
  http.post("/api/tickets/:id/actions", () =>
    HttpResponse.json(ticketFixture({ status: "in_review" })),
  ),
  http.get("/api/ticket-types", () => HttpResponse.json(page([]))),
];

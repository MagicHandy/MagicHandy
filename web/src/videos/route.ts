// `#/videos/<id>` opens one video, so a reload, a bookmark or a remote command
// can address it. Going back from the player returns to `#/videos`.
export function videoRoute(id: string): string {
  return id ? `#/videos/${encodeURIComponent(id)}` : "#/videos";
}

export function videoIDFromRoute(route: string): string {
  const [base, id] = route.replace(/^#\/?/, "").split("?")[0].split("/");
  if (base !== "videos" || !id) return "";
  try {
    return decodeURIComponent(id);
  } catch {
    return "";
  }
}

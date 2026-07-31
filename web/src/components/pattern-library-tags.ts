const HIDDEN_TAGS = new Set(["curated", "imported", "experimental"]);

export function visiblePatternTags(tags: string[]): string[] {
  return tags.filter((tag) => {
    const normalized = tag.trim().toLowerCase();
    if (!normalized || HIDDEN_TAGS.has(normalized)) {
      return false;
    }
    return !normalized.startsWith("pose-") && !normalized.startsWith("zone-");
  });
}

export function collectPatternTags(patterns: { tags: string[] }[]): string[] {
  const seen = new Set<string>();
  const ordered: string[] = [];
  for (const pattern of patterns) {
    for (const tag of visiblePatternTags(pattern.tags)) {
      if (seen.has(tag)) {
        continue;
      }
      seen.add(tag);
      ordered.push(tag);
    }
  }
  return ordered.sort((left, right) => left.localeCompare(right));
}

export function patternMatchesQuery(pattern: { name: string; description?: string | null; tags: string[] }, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return true;
  }
  const haystack = `${pattern.name} ${pattern.description ?? ""} ${visiblePatternTags(pattern.tags).join(" ")}`.toLowerCase();
  return haystack.includes(needle);
}

export function patternMatchesTags(pattern: { tags: string[] }, activeTags: ReadonlySet<string>): boolean {
  if (activeTags.size === 0) {
    return true;
  }
  const visible = visiblePatternTags(pattern.tags);
  return visible.some((tag) => activeTags.has(tag));
}

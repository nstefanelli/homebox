type TagLike = { id: string; name: string };

/** Case-insensitive, whitespace-trimmed match of an AI category hint against existing tags. */
export function matchHintToTag<T extends TagLike>(hint: string, tags: T[]): T | null {
  const needle = hint.trim().toLowerCase();
  if (!needle) {
    return null;
  }
  return tags.find(tag => tag.name.trim().toLowerCase() === needle) ?? null;
}

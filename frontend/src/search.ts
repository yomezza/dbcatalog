// A pipe separates AND conditions. Each condition remains a literal substring.
export function searchTerms(query: string): string[] {
  return [
    ...new Set(
      query
        .split("|")
        .map((s) => s.trim().toLocaleLowerCase("ru"))
        .filter(Boolean),
    ),
  ];
}
export function createSearchMatcher(
  query: string,
): (values: unknown[]) => boolean {
  const terms = searchTerms(query);
  return (values) => {
    const text = values
      .flat()
      .filter((v) => v != null)
      .join(" ")
      .toLocaleLowerCase("ru");
    return terms.every((term) => text.includes(term));
  };
}
export function matchesSearch(values: unknown[], query: string): boolean {
  return createSearchMatcher(query)(values);
}

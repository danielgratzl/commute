// Inline stroke icons (24x24). Rendered with {@html}; content is static.
const paths: Record<string, string> = {
  walk: '<circle cx="13" cy="4.5" r="1.8"/><path d="M10 21l2.2-6.2L15 17v4M8 12.5l2.4-3.6 3.4.6 2 3.4 2.7 1M12.2 14.8l1.3-5.4"/>',
  bus: '<rect x="5" y="3" width="14" height="15" rx="3"/><path d="M5 11h14M8 21v-3M16 21v-3"/>',
  train: '<rect x="6" y="3" width="12" height="14" rx="3.5"/><path d="M6 10h12M9 21l1.5-4M15 21l-1.5-4"/>',
  tram: '<rect x="6" y="6" width="12" height="12" rx="2.5"/><path d="M9 2.5h6M12 2.5V6M6 12h12M9 21l1-3M15 21l-1-3"/>',
  ship: '<path d="M3 15l2 5h14l2-5zM6 15V9h12v6M12 4v5"/>',
  cable: '<path d="M3 5l18-2M12 4v5"/><rect x="7" y="9" width="10" height="10" rx="2"/>',
  other: '<circle cx="12" cy="12" r="8"/>',
  swap: '<path d="M7 4L3 8l4 4M3 8h14M17 20l4-4-4-4M21 16H7"/>',
  warn: '<path d="M12 3l9.5 17h-19z"/><path d="M12 10v4.5M12 17.5v.2"/>',
  chev: '<path d="M6 9l6 6 6-6"/>',
  home: '<path d="M3 11l9-7 9 7"/><path d="M5 10v10h14V10"/><path d="M10 20v-5h4v5"/>',
  work: '<rect x="3" y="7" width="18" height="13" rx="2.5"/><path d="M9 7V5a2 2 0 012-2h2a2 2 0 012 2v2M3 13h18"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.2"/>',
}

export const icon = (name: string, size = 18) =>
  `<svg class="i" width="${size}" height="${size}" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">${paths[name] ?? paths.other}</svg>`

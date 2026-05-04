export interface SsePositionEvent {
  source: string;
  timestamp: string;
  eventType: string;
  X: number;
  Y: number;
  Z: number;
  Lat: number;
  Lng: number;
  Alt: number;
}

/** Ground segment positions are reported with zero altitude (km). */
export function isGroundStationEvent(ev: SsePositionEvent): boolean {
  return ev.Alt === 0;
}

export function sseEventsUrl(): string {
  return import.meta.env.VITE_SSE_URL ?? "/events-sse";
}

export interface SseNetworkLink {
  subject: string;
  ip: string;
  distance: number;
  packageDelay: number;
  packageLoss: number;
  bandwidth: number;
}

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
  networkParams?: SseNetworkLink[];
}

export interface SsePeerUsage {
  ip: string;
  peerFsNode?: string;
  totalBytesSent: number;
  totalBytesReceived: number;
  totalPacketsSent: number;
  totalPacketsReceived: number;
}

export interface SseAgentFileEvent {
  source: string;
  timestamp: string;
  eventType: "AgentFileEvent";
  action: "PUT" | "RECEIVED" | "DELETE" | string;
  fileName: string;
  contentSizeBytes?: number;
  md5?: string;
}

export interface SseNetworkUsageEvent {
  source: string;
  timestamp: string;
  eventType: "NetworkUsageEvent";
  TotalBytesSent: number;
  TotalBytesReceived: number;
  TotalPacketsSent: number;
  TotalPacketsReceived: number;
  peers?: SsePeerUsage[];
}

/** Ground segment positions are reported with zero altitude (km). */
export function isGroundStationEvent(ev: SsePositionEvent): boolean {
  return ev.Alt === 0;
}

export function sseEventsUrl(): string {
  return import.meta.env.VITE_SSE_URL ?? "/events-sse";
}

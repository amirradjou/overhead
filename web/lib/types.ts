// The JSON `overhead live` sends (internal/live: View, Plane; internal/events: Aircraft, Route, Photo).

export interface Airport {
  icao?: string;
  iata?: string;
  name?: string;
  city?: string;
  country?: string;
  lat?: number;
  lon?: number;
}

export interface Route {
  callsign: string;
  flight_iata?: string;
  airline?: string;
  airline_icao?: string;
  origin: Airport;
  destination: Airport;
}

export interface Photo {
  thumbnail: string;
  large?: string;
  link: string;
  photographer: string;
}

export interface Aircraft {
  hex: string;
  registration?: string;
  type_code?: string;
  type_name?: string;
  manufacturer?: string;
  owner?: string;
  owner_country?: string;
  route?: Route;
  photo?: Photo;
}

export interface Window {
  azimuth_deg: number;
  fov_deg: number;
  min_elevation_deg: number;
  home_elevation_m: number;
}

/** When and where a plane comes into view. */
export interface Coming {
  in_s: number;
  side: "left" | "right" | "rooftops" | "above" | "distance";
  offset_deg: number;
  elevation_deg: number;
}

export interface Plane {
  hex: string;
  callsign?: string;
  registration?: string;
  type_code?: string;
  category?: string;
  squawk?: string;
  emergency?: string;
  lat: number;
  lon: number;
  alt_ft?: number;
  on_ground?: boolean;
  speed_kt?: number;
  track_deg?: number;
  vert_rate_fpm?: number;
  military?: boolean;
  private?: boolean;
  source: string;
  at: string;
  distance_km: number;
  bearing_deg: number;
  info?: Aircraft;
  to_destination_km?: number;
  eta_min?: number;
  notable?: string;
  elevation_deg: number;
  offset_deg: number;
  in_view: boolean;
  coming?: Coming;
  /** Degrees right and up across the window over the next 10 s. */
  motion?: { offset_deg: number; elevation_deg: number };
}

export interface View {
  at: string;
  center: { lat: number; lon: number };
  radius_km: number;
  window: Window;
  planes: Plane[];
  today: number;
  sources: string[];
}

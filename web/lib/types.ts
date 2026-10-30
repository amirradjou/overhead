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
  /** The window's top edge as seen from where you sit; 90 means none. Older servers leave it out. */
  max_elevation_deg?: number;
  home_elevation_m: number;
}

/** What a plane is doing at the airport the screen follows (overhead live -airport). */
export interface AtAirport {
  /** By its route, or by how it moves; "ground" is moving on the airport's ground with no route to tell which. */
  op: "arriving" | "departing" | "ground";
  phase:
    "approach" | "final" | "landing" | "taxiing" | "arrived" | "gate" | "takeoff" | "climbing" | "runway";
  runway?: string;
  distance_km: number;
  /** Height above the airport, when airborne. */
  height_ft?: number;
  /** Minutes to touchdown (arriving), or since take-off (departing), at the speed it flies now. */
  in_min?: number;
  ago_min?: number;
}

/** The airport the screen follows, and its day. */
export interface FollowedAirport {
  icao: string;
  iata: string;
  name: string;
  lat: number;
  lon: number;
  elevation_ft: number;
  runway_in_use?: string;
  arrivals_today: number;
  departures_today: number;
}

/** The weather at home (Open-Meteo), when the live API has a fresh reading. */
export interface Weather {
  at: string;
  temp_c: number;
  /** WMO weather code. */
  code: number;
  /** Cloud cover in percent: all, low, mid and high. */
  cloud_cover: number;
  cloud_low: number;
  cloud_mid: number;
  cloud_high: number;
  visibility_m: number;
  wind_kmh: number;
  /** Where the wind blows from, degrees clockwise from north. */
  wind_from_deg: number;
  gust_kmh: number;
  pressure_hpa?: number;
  /** The airport whose weather report gave the sky and the wind, when one did. */
  station?: string;
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
  airport?: AtAirport;
  /** Very close to the window (within the view's near_km, in a straight line). */
  near?: boolean;
  /** One of the planes a focused screen shows. */
  focus?: boolean;
  /** The true altitude: alt_ft corrected for today's pressure, below 18,000 ft, when the pressure is known. */
  alt_msl_ft?: number;
}

export interface View {
  at: string;
  center: { lat: number; lon: number };
  radius_km: number;
  window: Window;
  planes: Plane[];
  today: number;
  sources: string[];
  /** Sentences about the history, for between planes. */
  facts: string[];
  /** Show only the planes marked focus: at the followed airport, or very close to the window. */
  focused?: boolean;
  near_km?: number;
  airport?: FollowedAirport;
  weather?: Weather;
}

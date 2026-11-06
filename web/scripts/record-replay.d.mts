import type { Recording } from "../lib/replay";
import type { View } from "../lib/types";

export function check(view: View): void;
export function compact(views: View[], stepS: number): Recording;

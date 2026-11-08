import { http } from "./client";

export type SubtitleAlignMode = "hardlink" | "copy";
export type SubtitleAlignActionKind = "link" | "exists" | "skip" | "conflict" | "error" | "remove" | "keep";

export interface SubtitleAlignSettings {
  enabled: boolean;
  mode: SubtitleAlignMode;
  fallback: boolean;
  sub_exts: string;
}

export interface SubtitleAlignAction {
  id: string;
  kind: SubtitleAlignActionKind;
  dir: string;
  video?: string;
  source: string;
  alias: string;
  mode?: string;
  reason?: string;
  status?: string;
  error?: string;
}

export interface SubtitleAlignPlan {
  id: string;
  root: string;
  created_at: string;
  actions: SubtitleAlignAction[];
}

const base = "/admin/tools/subtitle-align";
const longTimeoutMs = 300_000;

export function fetchSubtitleAlignSettings() {
  return http.get<SubtitleAlignSettings>(`${base}/settings`);
}

export function saveSubtitleAlignSettings(settings: SubtitleAlignSettings) {
  return http.put<SubtitleAlignSettings>(`${base}/settings`, settings);
}

export function scanSubtitleAlign(root: string, maxDepth: number) {
  return http.postWithTimeout<SubtitleAlignPlan>(`${base}/scan`, { root, max_depth: maxDepth }, longTimeoutMs);
}

export function applySubtitleAlign(plan: SubtitleAlignPlan) {
  return http.postWithTimeout<SubtitleAlignPlan>(`${base}/apply`, plan, longTimeoutMs);
}

export function cleanupSubtitleAlign(root: string, dryRun: boolean) {
  return http.postWithTimeout<SubtitleAlignPlan>(`${base}/cleanup`, { root, dry_run: dryRun }, longTimeoutMs);
}

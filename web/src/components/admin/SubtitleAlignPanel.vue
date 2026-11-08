<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import SettingsCard from "@/components/admin/SettingsCard.vue";
import SettingsRow from "@/components/admin/SettingsRow.vue";
import SettingsSegment from "@/components/admin/SettingsSegment.vue";
import SettingsBoolSegment from "@/components/admin/SettingsBoolSegment.vue";
import { getApiErrorMessage } from "@/api/client";
import {
  applySubtitleAlign,
  cleanupSubtitleAlign,
  fetchSubtitleAlignSettings,
  saveSubtitleAlignSettings,
  scanSubtitleAlign,
  type SubtitleAlignAction,
  type SubtitleAlignMode,
  type SubtitleAlignPlan,
} from "@/api/subtitleAlign";
import { toast } from "@/composables/useToast";

const ROOT_STORAGE_KEY = "litepan.subtitleAlign.root";
const DEPTH_STORAGE_KEY = "litepan.subtitleAlign.depth";
const MAX_ROWS = 500;

interface FormState {
  enabled: boolean;
  mode: string;
  fallback: boolean;
  sub_exts: string;
}

const DEFAULT_FORM: FormState = {
  enabled: true,
  mode: "hardlink",
  fallback: true,
  sub_exts: "srt,ass,ssa,vtt,sub,smi",
};

const root = ref("");
const maxDepth = ref("0");
const form = ref<FormState>({ ...DEFAULT_FORM });
const savedSnapshot = ref("");
const saving = ref(false);
const busy = ref<"" | "scan" | "apply" | "cleanup">("");
const plan = ref<SubtitleAlignPlan | null>(null);
const cleanupPlan = ref<SubtitleAlignPlan | null>(null);

const depthOptions = [
  { value: "0", label: "不限（递归全部子目录）" },
  { value: "1", label: "1 层" },
  { value: "2", label: "2 层" },
  { value: "3", label: "3 层" },
];

const modeOptions = [
  { value: "hardlink", label: "硬链接" },
  { value: "copy", label: "复制" },
];

const dirty = computed(() => JSON.stringify(form.value) !== savedSnapshot.value);
const displayPlan = computed(() => plan.value ?? cleanupPlan.value);
const displayActions = computed<SubtitleAlignAction[]>(() => displayPlan.value?.actions ?? []);
const visibleRows = computed(() => displayActions.value.slice(0, MAX_ROWS));
const hasLinks = computed(() => plan.value?.actions.some((a) => a.kind === "link") ?? false);

const counts = computed(() => {
  const out: Record<SubtitleAlignActionKindKey, number> = {
    link: 0,
    exists: 0,
    conflict: 0,
    skip: 0,
    error: 0,
    remove: 0,
    keep: 0,
  };
  for (const a of displayActions.value) {
    if (a.kind in out) out[a.kind as SubtitleAlignActionKindKey] += 1;
  }
  return out;
});

type SubtitleAlignActionKindKey = "link" | "exists" | "conflict" | "skip" | "error" | "remove" | "keep";

const countChips = computed(() => {
  const list: { key: SubtitleAlignActionKindKey; label: string; count: number }[] = [
    { key: "link", label: "待创建", count: counts.value.link },
    { key: "exists", label: "已存在", count: counts.value.exists },
    { key: "conflict", label: "冲突", count: counts.value.conflict },
    { key: "skip", label: "跳过", count: counts.value.skip },
    { key: "error", label: "失败", count: counts.value.error },
    { key: "remove", label: "待清理", count: counts.value.remove },
    { key: "keep", label: "保留", count: counts.value.keep },
  ];
  return list.filter((item) => item.count > 0);
});

function readStored(key: string): string {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

function writeStored(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* 隐私模式下忽略 */
  }
}

function labelOf(a: SubtitleAlignAction): string {
  if (a.status === "done") return a.kind === "remove" ? "已清理" : "已创建";
  if (a.status === "failed") return "失败";
  const labels: Record<string, string> = {
    link: "待创建",
    exists: "已存在",
    skip: "跳过",
    conflict: "冲突",
    error: "失败",
    remove: "待清理",
    keep: "保留",
  };
  return labels[a.kind] ?? a.kind;
}

function toneOf(a: SubtitleAlignAction): string {
  if (a.status === "failed" || a.kind === "error") return "danger";
  if (a.status === "done" || a.kind === "exists") return "success";
  if (a.kind === "conflict" || a.kind === "remove") return "warn";
  if (a.kind === "link") return "brand";
  return "muted";
}

function reasonOf(a: SubtitleAlignAction): string {
  return a.error || a.reason || (a.mode ? `方式：${a.mode === "copy" ? "复制" : "硬链接"}` : "");
}

async function loadSettings() {
  try {
    const data = await fetchSubtitleAlignSettings();
    form.value = {
      enabled: data.enabled,
      mode: data.mode,
      fallback: data.fallback,
      sub_exts: data.sub_exts,
    };
    savedSnapshot.value = JSON.stringify(form.value);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "读取字幕对齐设置失败"));
  }
}

async function saveSettings(): Promise<boolean> {
  if (!dirty.value) return true;
  saving.value = true;
  try {
    const saved = await saveSubtitleAlignSettings({
      enabled: form.value.enabled,
      mode: form.value.mode as SubtitleAlignMode,
      fallback: form.value.fallback,
      sub_exts: form.value.sub_exts,
    });
    form.value = {
      enabled: saved.enabled,
      mode: saved.mode,
      fallback: saved.fallback,
      sub_exts: saved.sub_exts,
    };
    savedSnapshot.value = JSON.stringify(form.value);
    toast.success("设置已保存");
    return true;
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存设置失败"));
    return false;
  } finally {
    saving.value = false;
  }
}

async function scan() {
  const target = root.value.trim();
  if (!target) {
    toast.warning("请填写要扫描的媒体目录");
    return;
  }
  if (!(await saveSettings())) return;
  busy.value = "scan";
  try {
    plan.value = await scanSubtitleAlign(target, Number(maxDepth.value) || 0);
    cleanupPlan.value = null;
    if (!plan.value.actions.length) {
      toast.info("没有找到可对齐的字幕目录");
    } else {
      toast.success(`扫描完成，共 ${plan.value.actions.length} 项`);
    }
  } catch (e) {
    toast.error(getApiErrorMessage(e, "扫描失败"));
  } finally {
    busy.value = "";
  }
}

async function apply() {
  if (!plan.value) return;
  busy.value = "apply";
  try {
    plan.value = await applySubtitleAlign(plan.value);
    const done = plan.value.actions.filter((a) => a.status === "done").length;
    const failed = plan.value.actions.filter((a) => a.status === "failed").length;
    if (failed > 0) {
      toast.warning(`已创建 ${done} 个，失败 ${failed} 个，详见列表`);
    } else if (done > 0) {
      toast.success(`已创建 ${done} 个别名`);
    } else {
      toast.info("没有需要新建的别名");
    }
  } catch (e) {
    toast.error(getApiErrorMessage(e, "应用失败"));
  } finally {
    busy.value = "";
  }
}

async function validate() {
  const target = root.value.trim();
  if (!target) {
    toast.warning("请填写要校验的媒体目录");
    return;
  }
  busy.value = "cleanup";
  try {
    cleanupPlan.value = await cleanupSubtitleAlign(target, true);
    plan.value = null;
    const removable = cleanupPlan.value.actions.filter((a) => a.kind === "remove").length;
    toast.info(`校验完成：${removable} 个别名可安全清理`);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "校验失败"));
  } finally {
    busy.value = "";
  }
}

async function cleanup() {
  const target = root.value.trim();
  if (!target) {
    toast.warning("请填写要清理的媒体目录");
    return;
  }
  busy.value = "cleanup";
  try {
    cleanupPlan.value = await cleanupSubtitleAlign(target, false);
    plan.value = null;
    const removed = cleanupPlan.value.actions.filter((a) => a.status === "done").length;
    const kept = cleanupPlan.value.actions.filter((a) => a.kind === "keep").length;
    toast.success(`已清理 ${removed} 个别名${kept ? `，保留 ${kept} 个` : ""}`);
  } catch (e) {
    toast.error(getApiErrorMessage(e, "清理失败"));
  } finally {
    busy.value = "";
  }
}

watch(root, (value) => writeStored(ROOT_STORAGE_KEY, value));
watch(maxDepth, (value) => writeStored(DEPTH_STORAGE_KEY, value));

onMounted(() => {
  root.value = readStored(ROOT_STORAGE_KEY) || "/app/strm";
  maxDepth.value = readStored(DEPTH_STORAGE_KEY) || "0";
  void loadSettings();
});
</script>

<template>
  <div class="subtitle-align">
    <SettingsCard title="扫描设置">
      <template #head-actions>
        <AppButton type="button" variant="secondary" :disabled="saving || busy !== ''" @click="saveSettings">
          {{ dirty ? "保存设置" : "已保存" }}
        </AppButton>
      </template>

      <SettingsRow>
        <template #info><span class="settings-row__label">媒体目录</span></template>
        <template #control>
          <AppInput v-model="root" placeholder="/app/strm/电影" />
        </template>
      </SettingsRow>

      <SettingsRow>
        <template #info><span class="settings-row__label">扫描深度</span></template>
        <template #control>
          <AppSelect v-model="maxDepth" :options="depthOptions" />
        </template>
      </SettingsRow>

      <SettingsRow>
        <template #info><span class="settings-row__label">链接方式</span></template>
        <template #control>
          <SettingsSegment v-model="form.mode" label="链接方式" :options="modeOptions" />
        </template>
      </SettingsRow>

      <SettingsRow>
        <template #info><span class="settings-row__label">硬链接失败时降级复制</span></template>
        <template #control>
          <SettingsBoolSegment v-model="form.fallback" label="硬链接失败时降级复制" />
        </template>
      </SettingsRow>

      <SettingsRow>
        <template #info><span class="settings-row__label">字幕扩展名</span></template>
        <template #control>
          <AppInput v-model="form.sub_exts" placeholder="srt,ass,ssa" />
        </template>
      </SettingsRow>
    </SettingsCard>

    <SettingsCard title="对齐计划">
      <template #head-actions>
        <AppButton type="button" variant="primary" :disabled="busy !== ''" @click="scan">
          {{ busy === "scan" ? "扫描中…" : "扫描" }}
        </AppButton>
        <AppButton type="button" variant="primary" :disabled="!hasLinks || busy !== ''" @click="apply">
          {{ busy === "apply" ? "应用中…" : "应用" }}
        </AppButton>
        <AppButton type="button" variant="secondary" :disabled="busy !== ''" @click="validate">校验</AppButton>
        <AppButton type="button" variant="secondary" :disabled="busy !== ''" @click="cleanup">清理</AppButton>
      </template>

      <div v-if="displayPlan" class="sa-summary">
        <span class="sa-summary__total">共 {{ displayActions.length }} 项</span>
        <span v-for="chip in countChips" :key="chip.key" class="sa-summary__chip">
          {{ chip.label }} {{ chip.count }}
        </span>
      </div>

      <div v-if="visibleRows.length" class="sa-table-wrap">
        <table class="sa-table">
          <thead>
            <tr>
              <th>状态</th>
              <th>视频</th>
              <th>源字幕</th>
              <th>生成文件</th>
              <th>说明</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="a in visibleRows" :key="a.id || a.alias">
              <td>
                <span class="sa-badge" :class="`sa-badge--${toneOf(a)}`">{{ labelOf(a) }}</span>
              </td>
              <td class="sa-mono">{{ a.video || "—" }}</td>
              <td class="sa-mono">{{ a.source }}</td>
              <td class="sa-mono">{{ a.alias }}</td>
              <td class="sa-reason">{{ reasonOf(a) }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="displayActions.length > visibleRows.length" class="sa-more">
          仅显示前 {{ MAX_ROWS }} 项，共 {{ displayActions.length }} 项
        </p>
      </div>

      <p v-else class="sa-empty">
        填写媒体目录后点「扫描」，这里会列出将被创建、已存在冲突或可清理的字幕别名。
      </p>

      <p class="sa-hint">
        规则：源字幕名 = 所在目录名；目录内以目录名开头的 .strm 视为版本视频。清理只删除本工具创建、且仍指向源字幕的别名。
      </p>
    </SettingsCard>
  </div>
</template>

<style scoped>
.subtitle-align {
  display: flex;
  flex-direction: column;
}

.sa-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 12px 0;
  font-size: 13px;
  color: var(--text-muted);
}

.sa-summary__total {
  font-weight: 600;
  color: var(--text);
}

.sa-summary__chip {
  padding: 2px 10px;
  border-radius: var(--radius-pill);
  background: var(--border-soft);
}

.sa-table-wrap {
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-sm);
  overflow: auto;
  max-height: 460px;
  margin-bottom: 12px;
}

.sa-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.sa-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  text-align: left;
  font-weight: 600;
  color: var(--text-muted);
  background: var(--surface);
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-soft);
  white-space: nowrap;
}

.sa-table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border-soft);
  vertical-align: top;
}

.sa-table tr:last-child td {
  border-bottom: none;
}

.sa-mono {
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Consolas, monospace);
  word-break: break-all;
}

.sa-reason {
  color: var(--text-muted);
  word-break: break-word;
}

.sa-badge {
  display: inline-block;
  padding: 1px 8px;
  border-radius: var(--radius-pill);
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}

.sa-badge--brand {
  color: var(--brand);
  background: color-mix(in srgb, var(--brand) 12%, transparent);
}

.sa-badge--success {
  color: var(--success, #16a34a);
  background: color-mix(in srgb, var(--success, #16a34a) 12%, transparent);
}

.sa-badge--warn {
  color: var(--warning, #b45309);
  background: color-mix(in srgb, var(--warning, #b45309) 14%, transparent);
}

.sa-badge--danger {
  color: var(--danger, #dc2626);
  background: color-mix(in srgb, var(--danger, #dc2626) 12%, transparent);
}

.sa-badge--muted {
  color: var(--text-muted);
  background: var(--border-soft);
}

.sa-empty {
  padding: 24px 0;
  text-align: center;
  color: var(--text-muted);
}

.sa-more {
  padding: 8px 12px;
  margin: 0;
  color: var(--text-muted);
  font-size: 12px;
}

.sa-hint {
  margin: 0 0 16px;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.6;
}
</style>

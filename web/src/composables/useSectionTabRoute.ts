import { ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAdminActivePage } from "./useAdminLoadingBar";

export function useSectionTabRoute(
  defaultTab: string,
  validTabs: readonly string[],
  options?: {
    beforeTabChange?: (from: string, to: string) => Promise<boolean>;
    // 该子页面所属的后台页面 key（route.query.page）。
    // 设置了之后，只有当前 page 匹配时才读写 tab，避免 KeepAlive 缓存的其它页面互相改写。
    pageKey?: string;
  },
) {
  const route = useRoute();
  const router = useRouter();
  const activePage = useAdminActivePage();

  function pageActive(): boolean {
    const pageKey = options?.pageKey;
    if (!pageKey) return true;
    // 路由里的 page 优先（页面切换时父组件的 activePage 可能晚一拍）；
    // 没有 page 查询（后台首页默认页）时再回退到 AdminView 的当前页面。
    const routePage = String(route.query.page ?? "").trim();
    if (routePage) return routePage === pageKey;
    if (activePage) return activePage.value === pageKey;
    return false;
  }

  function normalizeTabId(tab: string): string {
    const trimmed = tab.trim();
    if (!trimmed) return defaultTab;
    return validTabs.includes(trimmed) ? trimmed : defaultTab;
  }

  const activeTab = ref(normalizeTabId(String(route.query.tab ?? "")));
  let syncingFromRoute = false;

  async function setActiveTab(next: string): Promise<boolean> {
    const target = normalizeTabId(next);
    const from = activeTab.value;
    if (target === from) return true;
    if (options?.beforeTabChange) {
      const ok = await options.beforeTabChange(from, target);
      if (!ok) return false;
    }
    activeTab.value = target;
    return true;
  }

  watch(activeTab, (val) => {
    if (syncingFromRoute) return;
    const normalized = normalizeTabId(val);
    if (normalized !== val) {
      activeTab.value = normalized;
      return;
    }
    if (!pageActive()) return;
    if (!val || String(route.query.tab ?? "") === val) return;
    // 用户主动切换 Tab 应写入浏览器历史，使前进/后退能还原上一个视图。
    void router.push({ query: { ...route.query, tab: val } });
  });

  watch(
    () => route.query.tab,
    (q) => {
      if (!pageActive()) return;
      const target = normalizeTabId(String(q ?? ""));
      if (target === activeTab.value) return;
      syncingFromRoute = true;
      void (async () => {
        const changed = await setActiveTab(target);
        syncingFromRoute = false;
        if (!changed && String(route.query.tab ?? "") !== activeTab.value) {
          await router.replace({ query: { ...route.query, tab: activeTab.value } });
        }
      })();
    },
  );

  return { activeTab, setActiveTab };
}

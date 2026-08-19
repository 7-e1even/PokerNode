import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { api } from "./api";
import AuthScreen from "./AuthScreen";
import AccountBindings from "./AccountBindings";
import ChannelRoom from "./ChannelRoom";
import Dashboard from "./Dashboard";
import BalanceManager from "./BalanceManager";
import { Spinner } from "@/components/ui/spinner";
import { BrandMark } from "@/components/brand-mark";
import type { AdminSection } from "@/components/app-sidebar";
import type { Space, User } from "./types";

const AdminAuthScreen = lazy(() => import("./AdminAuthScreen"));
const AdminDashboard = lazy(() => import("./AdminDashboard"));

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [selectedSpace, setSelectedSpace] = useState<Space | null>(null);
  const [registrationEnabled, setRegistrationEnabled] = useState(true);
  const [wechatLoginEnabled, setWechatLoginEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [routeLoading, setRouteLoading] = useState(false);
  const [routeError, setRouteError] = useState("");
  const [path, setPath] = useState(window.location.pathname);
  const route = useMemo(() => parseRoute(path), [path]);

  const navigate = useCallback((nextPath: string, replace = false) => {
    window.history[replace ? "replaceState" : "pushState"]({}, "", nextPath);
    setPath(window.location.pathname);
  }, []);

  useEffect(() => {
    Promise.allSettled([
      api<{ user: User }>("/api/me"),
      api<{ registration_enabled: boolean; wechat_login_enabled: boolean }>("/api/config"),
    ]).then(([session, config]) => {
      setUser(session.status === "fulfilled" ? session.value.user : null);
      if (config.status === "fulfilled") {
        setRegistrationEnabled(config.value.registration_enabled);
        setWechatLoginEnabled(config.value.wechat_login_enabled);
      }
    }).finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    const onPopState = () => setPath(window.location.pathname);
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  useEffect(() => {
    if (route.page === "not_found") navigate("/", true);
    if (route.page === "admin_login" && hasPermission(user, "admin:view")) navigate("/admin", true);
  }, [navigate, route.page, user]);

  useEffect(() => {
    if (!user || (route.page !== "channel" && route.page !== "channel_balances")) return;
    if (selectedSpace?.id === route.channelID) {
      setRouteLoading(false);
      setRouteError("");
      return;
    }
    let cancelled = false;
    setRouteLoading(true);
    setRouteError("");
    api<{ space: Space }>(`/api/spaces/${route.channelID}`)
      .then((result) => {
        if (!cancelled) setSelectedSpace(result.space);
      })
      .catch((caught) => {
        if (!cancelled) setRouteError(caught instanceof Error ? caught.message : "频道加载失败");
      })
      .finally(() => {
        if (!cancelled) setRouteLoading(false);
      });
    return () => { cancelled = true; };
  }, [route, selectedSpace?.id, user]);

  if (loading) {
    return (
      <main className="grid min-h-svh place-items-center bg-muted/30">
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <BrandMark className="size-9" />
          <Spinner />
          <span>正在打开牌桌…</span>
        </div>
      </main>
    );
  }

  if (route.page === "admin" || route.page === "admin_login") {
    if (!user || !hasPermission(user, "admin:view")) {
      return <Suspense fallback={<RouteLoading />}><AdminAuthScreen currentUser={user} onBack={() => navigate("/")} onAuthenticated={(nextUser) => { setUser(nextUser); navigate("/admin", true); }} /></Suspense>;
    }
    return (
      <Suspense fallback={<RouteLoading />}>
        <AdminDashboard
          user={user}
          section={route.page === "admin" ? route.section : "overview"}
          onSectionChanged={(section) => navigate(adminPath(section))}
          onOpenLobby={() => navigate("/")}
          onUserUpdated={setUser}
          onRegistrationChanged={setRegistrationEnabled}
          onLogout={() => { setUser(null); navigate("/admin/login", true); }}
        />
      </Suspense>
    );
  }

  if (!user) return <AuthScreen registrationEnabled={registrationEnabled} wechatLoginEnabled={wechatLoginEnabled} onAuthenticated={setUser} />;

  if (route.page === "account_bindings") {
    return <AccountBindings onBack={() => navigate("/")} onOpenSpace={(spaceID) => navigate(`/channels/${spaceID}`)} />;
  }

  if (route.page === "channel" || route.page === "channel_balances") {
    if (routeError) {
      return (
        <main className="grid min-h-svh place-items-center bg-muted/30 p-6">
          <Alert variant="destructive" className="max-w-md"><AlertTitle>无法打开频道</AlertTitle><AlertDescription>{routeError}</AlertDescription><Button className="mt-4" variant="outline" onClick={() => navigate("/")}>返回大厅</Button></Alert>
        </main>
      );
    }
    if (routeLoading || !selectedSpace || selectedSpace.id !== route.channelID) return <RouteLoading />;
    if (route.page === "channel_balances") {
      if (!selectedSpace.is_owner && !hasPermission(user, "balances:manage")) {
        return <main className="grid min-h-svh place-items-center bg-muted/30 p-6"><Alert variant="destructive" className="max-w-md"><AlertTitle>无法管理余额</AlertTitle><AlertDescription>你没有该频道的管理权限。</AlertDescription><Button className="mt-4" variant="outline" onClick={() => navigate(`/channels/${selectedSpace.id}`)}>返回频道</Button></Alert></main>;
      }
      return <BalanceManager spaces={[{ id: selectedSpace.id, name: selectedSpace.name }]} initialSpaceID={selectedSpace.id} onBack={() => navigate(`/channels/${selectedSpace.id}`)} />;
    }
    return (
      <ChannelRoom
        user={user}
        initialSpace={selectedSpace}
        initialTableID={route.tableID}
        onBack={() => navigate("/")}
        onNavigateTable={(tableID) => navigate(tableID ? `/channels/${selectedSpace.id}/tables/${tableID}` : `/channels/${selectedSpace.id}`)}
        onOpenBindings={() => navigate("/account/bindings")}
        onOpenBalances={() => navigate(`/channels/${selectedSpace.id}/balances`)}
      />
    );
  }

  return (
    <Dashboard
      user={user}
      wechatLoginEnabled={wechatLoginEnabled}
      onOpenBindings={() => navigate("/account/bindings")}
      onOpenSpace={(space) => {
        setSelectedSpace(space);
        navigate(`/channels/${space.id}`);
      }}
      onOpenAdmin={hasPermission(user, "admin:view") ? () => navigate("/admin") : undefined}
      onUserUpdated={setUser}
      onLogout={() => {
        setSelectedSpace(null);
        setUser(null);
        navigate("/", true);
      }}
    />
  );
}

type AppRoute =
  | { page: "lobby" }
  | { page: "account_bindings" }
  | { page: "admin_login" }
  | { page: "admin"; section: AdminSection }
  | { page: "channel"; channelID: string; tableID?: string }
  | { page: "channel_balances"; channelID: string }
  | { page: "not_found" };

function parseRoute(pathname: string): AppRoute {
  const parts = pathname.split("/").filter(Boolean).map(decodeURIComponent);
  if (parts.length === 0) return { page: "lobby" };
  if (parts.length === 2 && parts[0] === "account" && parts[1] === "bindings") return { page: "account_bindings" };
  if (parts[0] === "admin") {
    if (parts.length === 1) return { page: "admin", section: "overview" };
    if (parts.length === 2 && parts[1] === "login") return { page: "admin_login" };
    if (parts.length === 2 && (["users", "roles", "channels", "balances", "rankings", "settings"] as string[]).includes(parts[1])) return { page: "admin", section: parts[1] as AdminSection };
  }
  if (parts[0] === "channels" && parts[1]) {
    if (parts.length === 2) return { page: "channel", channelID: parts[1] };
    if (parts.length === 3 && parts[2] === "balances") return { page: "channel_balances", channelID: parts[1] };
    if (parts.length === 4 && parts[2] === "tables" && parts[3]) return { page: "channel", channelID: parts[1], tableID: parts[3] };
  }
  return { page: "not_found" };
}

function adminPath(section: AdminSection) {
  return section === "overview" ? "/admin" : `/admin/${section}`;
}

function hasPermission(user: User | null, permission: string) {
  return !!user?.permissions?.includes(permission);
}

function RouteLoading() {
  return (
    <main className="grid min-h-svh place-items-center bg-muted/30">
      <div className="flex items-center gap-3 text-sm text-muted-foreground"><Spinner /><span>正在恢复频道页面…</span></div>
    </main>
  );
}

import { Component, type ErrorInfo, type ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";

interface Props {
  children: ReactNode;
}

interface State {
  failed: boolean;
}

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("PokerNode UI render failed", error, info);
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <main className="grid min-h-svh place-items-center bg-muted/30 p-6">
        <Alert variant="destructive" className="max-w-md">
          <AlertTitle>页面显示异常</AlertTitle>
          <AlertDescription>当前页面没有正确渲染。刷新后仍有问题时，请返回大厅重新进入。</AlertDescription>
          <div className="mt-4 flex flex-wrap gap-2">
            <Button onClick={() => window.location.reload()}>刷新页面</Button>
            <Button variant="outline" onClick={() => window.location.assign("/")}>返回大厅</Button>
          </div>
        </Alert>
      </main>
    );
  }
}

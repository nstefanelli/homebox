import { PublicApi } from "~~/lib/api/public";
import { UserClient } from "~~/lib/api/user";
import { Requests } from "~~/lib/requests";

export type Observer = {
  handler: (r: Response, req?: RequestInit) => void;
};

export type RemoveObserver = () => void;

const observers: Record<string, Observer> = {};

export function defineObserver(key: string, observer: Observer): RemoveObserver {
  observers[key] = observer;

  return () => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete
    delete observers[key];
  };
}

export function usePublicApi(): PublicApi {
  const requests = new Requests("", "", {});
  return new PublicApi(requests);
}

export function useUserApi(): UserClient {
  const authCtx = useAuthContext();
  const prefs = useViewPreferences();

  // The collection can change while long-lived clients (Pinia stores and the
  // singleton create dialog) remain mounted. Resolve X-Tenant for every
  // request so those clients cannot keep writing to the previously selected
  // collection.
  const requests = new Requests("", "", (): Record<string, string> => {
    const collectionId = prefs?.value?.collectionId;
    return collectionId ? { "X-Tenant": collectionId } : {};
  });
  requests.addResponseInterceptor(async r => {
    if (r.status === 401) {
      console.error("unauthorized request, invalidating session");
      authCtx.invalidateSession();
      navigateTo("/");
    }

    if (r.status === 403) {
      try {
        const contentType = r.headers.get("Content-Type") ?? "";
        if (!contentType.startsWith("application/json")) {
          return;
        }

        const body = (await r.json().catch(() => null)) as { error?: string } | null;

        if (
          body?.error === "user does not have access to the requested tenant" &&
          window.location.pathname !== "/" &&
          prefs?.value?.collectionId
        ) {
          prefs.value.collectionId = null;
        }
      } catch {
        // ignore parsing errors to avoid breaking the interceptor chain
      }
    }
  });

  for (const [_, observer] of Object.entries(observers)) {
    requests.addResponseInterceptor(observer.handler);
  }

  return new UserClient(requests, authCtx.attachmentToken || "");
}

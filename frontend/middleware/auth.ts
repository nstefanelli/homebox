export default defineNuxtRouteMiddleware(async () => {
  const ctx = useAuthContext();
  const api = useUserApi();
  const redirectTo = useState("authRedirect");

  if (!ctx.isAuthorized()) {
    if (window.location.pathname !== "/") {
      redirectTo.value = window.location.pathname;
      return navigateTo("/");
    }
  }

  if (!ctx.user) {
    const { data, error } = await api.user.self();
    if (error) {
      if (window.location.pathname !== "/") {
        redirectTo.value = window.location.pathname;
        return navigateTo("/");
      }
    }

    ctx.user = data.item;
  }
});

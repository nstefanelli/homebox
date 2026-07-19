try {
  const preferences = JSON.parse(localStorage.getItem("homebox/preferences/location") || "{}");
  const theme = preferences?.theme;
  if (theme) {
    document.documentElement.setAttribute("data-theme", theme);
    document.documentElement.classList.add("theme-" + theme);
  }
} catch (e) {
  console.error("Failed to set theme", e);
}

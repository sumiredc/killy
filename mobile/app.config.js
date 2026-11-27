// app.json is the Expo config. This changes it for one build only: the Web version for the public
// demo (EXPO_PUBLIC_KILLY_WEB_DEMO=1), which the introduction site serves under /demo/. A base URL
// is for the Web, but Expo's native release bundling reads it as well, so the phone builds (EAS)
// never get it: they do not set the variable.
module.exports = ({ config }) => {
  if (process.env.EXPO_PUBLIC_KILLY_WEB_DEMO?.trim() !== '1') {
    return config;
  }
  return { ...config, experiments: { ...config.experiments, baseUrl: '/demo' } };
};

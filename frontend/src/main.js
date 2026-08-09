import Vue from 'vue';
import Buefy from 'buefy';
import VueI18n from 'vue-i18n';

import App from './App.vue';
import router from './router';
import store from './store';
import * as api from './api';
import Utils from './utils';

// Internationalisation.
Vue.use(VueI18n);
const i18n = new VueI18n();

Vue.use(Buefy, {});
Vue.config.productionTip = false;

// Setup the router.
router.beforeEach((to, from, next) => {
  if (to.matched.length === 0) {
    next('/404');
  } else {
    next();
  }
});

router.afterEach((to) => {
  Vue.nextTick(() => {
    const t = to.meta.title && i18n.te(to.meta.title) ? `${i18n.tc(to.meta.title, 0)} /` : '';
    document.title = `${t} listmonk`;
  });
});

async function initConfig(app) {
  // Load logged in user profile, server side config, and the language file before mounting the app.
  const [profile, cfg] = await Promise.all([api.getUserProfile(), api.getServerConfig()]);

  const lang = await api.getLang(cfg.lang);
  i18n.locale = cfg.lang;
  i18n.setLocaleMessage(i18n.locale, lang);

  Vue.prototype.$utils = new Utils(i18n);
  Vue.prototype.$api = api;
  Vue.prototype.$events = app;

  // $can('permission:name') is used in the UI to check whether the logged in user
  // has a certain permission to toggle visibility of UI objects and UI functionality.
  Vue.prototype.$can = (...perms) => {
    const tenantPermissions = profile.mailviewPermissions || [];
    if (!profile.mailviewTenantId && profile.userRole.id === 1) {
      return true;
    }

    // If the perm ends with a wildcard, check whether at least one permission
    // in the group is present. Eg: campaigns:* will return true if at least
    // one of campaigns:get, campaigns:manage etc. are present.
    const tenantMap = {
      'subscribers:get': 'subscriber.read.tenant',
      'subscribers:get_all': 'subscriber.read.tenant',
      'subscribers:manage': 'subscriber.manage.tenant',
      'subscribers:import': 'subscriber.import.tenant',
      'lists:get_all': 'list.read.tenant',
      'lists:manage': 'list.manage.tenant',
      'lists:manage_all': 'list.manage.tenant',
      'campaigns:get': 'campaign.read.tenant',
      'campaigns:get_all': 'campaign.read.tenant',
      'campaigns:manage': 'campaign.manage.tenant',
      'campaigns:manage_all': 'campaign.manage.tenant',
      'campaigns:send': 'campaign.send.tenant',
      'campaigns:get_analytics': 'analytics.read.tenant',
      'templates:get': 'template.read.tenant',
      'templates:manage': 'template.manage.tenant',
      'media:get': 'media.read.tenant',
      'media:manage': 'media.manage.tenant',
      'bounces:get': 'bounce.read.tenant',
      'bounces:manage': 'bounce.manage.tenant',
    };
    return perms.some((perm) => {
      if (perm.endsWith('*')) {
        const group = `${perm.split(':')[0]}:`;
        const legacy = profile.userRole.permissions.some((p) => p.startsWith(group));
        const tenant = Object.keys(tenantMap).some((p) => p.startsWith(group)
          && tenantPermissions.includes(tenantMap[p]));
        return profile.mailviewTenantId ? tenant : (legacy || tenant);
      }

      const tenant = tenantMap[perm] && tenantPermissions.includes(tenantMap[perm]);
      return profile.mailviewTenantId ? tenant : (profile.userRole.permissions.includes(perm) || tenant);
    });
  };

  Vue.prototype.$canList = (id, perm) => {
    if (!profile.mailviewTenantId && profile.userRole.id === 1) {
      return true;
    }

    // If the user role has global list permissions, return true.
    const can = Vue.prototype.$can('lists:get_all', 'lists:manage_all');
    if (can) {
      return true;
    }

    return Boolean(profile.listRole && profile.listRole.lists
      && profile.listRole.lists.some((list) => list.id === id && list.permissions.includes(perm)));
  };

  // Platform roles are independent from tenant memberships and Listmonk
  // roles. The primordial Super Admin remains a compatibility bridge.
  Vue.prototype.$canPlatform = (...perms) => {
    if (profile.mailviewTenantId) return false;
    if (profile.userRole.id === 1) return true;
    const platformPermissions = profile.mailviewPlatformPermissions || [];
    return perms.some((perm) => platformPermissions.includes(perm));
  };

  // Set the page title after i18n has loaded.
  const to = router.history.current;
  const title = to.meta.title ? `${i18n.tc(to.meta.title, 0)} /` : '';
  document.title = `${title} listmonk`;

  if (app) {
    app.$mount('#app');
  }
}

const v = new Vue({
  router,
  store,
  i18n,
  render: (h) => h(App),

  data: {
    isLoaded: false,
  },

  methods: {
    loadConfig() {
      initConfig();
    },

    // awaitRestart handles app restart polling after settings changes.
    // Shows a toast and polls until the backend is back up.
    // Returns a promise that resolves with { needsRestart: boolean }.
    awaitRestart(response) {
      return new Promise((resolve) => {
        // If there are running campaigns, app won't auto restart.
        if (response && typeof response === 'object' && response.needsRestart) {
          this.loadConfig();
          resolve({ needsRestart: true });
          return;
        }

        Vue.prototype.$utils.toast(i18n.t('settings.messengers.messageSaved'));

        // Poll until backend is back up.
        const pollId = setInterval(() => {
          api.getHealth().then(() => {
            clearInterval(pollId);
            this.loadConfig();
            resolve({ needsRestart: false });
          });
        }, 1000);
      });
    },
  },

  mounted() {
    v.isLoaded = true;
  },
});

initConfig(v);

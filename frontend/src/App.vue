<template>
  <div id="app">
    <b-navbar :fixed-top="true" v-if="$root.isLoaded">
      <template #brand>
        <div class="logo">
          <router-link :to="{ name: 'dashboard' }">
            <img class="full" src="@/assets/logo.svg" alt="" />
            <img class="favicon" src="@/assets/favicon.png" alt="" />
          </router-link>
        </div>
      </template>
      <template #end>
        <navigation v-if="isMobile" :is-mobile="isMobile" :active-item="activeItem" :active-group="activeGroup"
          @toggleGroup="toggleGroup" @doLogout="doLogout" />

        <b-navbar-item tag="a" href="#" @click.prevent="emitPageRefresh" data-cy="btn-refresh"
          :aria-label="$t('globals.buttons.refresh')">
          <b-tooltip :label="$t('globals.buttons.refresh')" type="is-dark" position="is-bottom">
            <b-icon icon="refresh" /> <span class="is-hidden-tablet">{{ $t('globals.buttons.refresh') }}</span>
          </b-tooltip>
        </b-navbar-item>

        <b-navbar-dropdown class="user" tag="div" right>
          <template v-if="profile.username" #label>
            <span class="user-avatar">
              <img v-if="profile.avatar" :src="profile.avatar" alt="" />
              <span v-else>{{ profile.username[0].toUpperCase() }}</span>
            </span>
          </template>

          <b-navbar-item class="user-name" tag="router-link" to="/user/profile">
            <strong>{{ profile.username }}</strong>
            <div class="is-size-7">{{ profile.name }}</div>
          </b-navbar-item>

          <b-navbar-item href="#">
            <router-link to="/user/profile">
              <b-icon icon="account-outline" /> {{ $t('users.profile') }}
            </router-link>
          </b-navbar-item>
          <b-navbar-item href="#">
            <a href="#" @click.prevent="doLogout"><b-icon icon="logout-variant" /> {{ $t('users.logout') }}</a>
          </b-navbar-item>
        </b-navbar-dropdown>
      </template>
    </b-navbar>

    <div class="wrapper" v-if="$root.isLoaded">
      <section class="sidebar">
        <b-sidebar position="static" mobile="hide" :fullheight="true" :open="true" :can-cancel="false">
          <div>
            <b-menu :accordion="false">
              <navigation v-if="!isMobile" :is-mobile="isMobile" :active-item="activeItem" :active-group="activeGroup"
                @toggleGroup="toggleGroup" />
            </b-menu>
          </div>
        </b-sidebar>
      </section>
      <!-- sidebar-->

      <!-- body //-->
      <div class="main">
        <div v-if="profile.mailviewTenantId" class="tenant-context-banner">
          <span class="tenant-context-banner__icon"><b-icon icon="check-circle-outline" /></span>
          <span class="tenant-context-banner__copy">
            <small>AMBIENTE DO CLIENTE</small>
            <strong>{{ tenantHostname }}</strong>
          </span>
          <span class="tenant-context-banner__boundary">
            <span>Tenant isolado</span>
            <code>{{ shortActiveTenantId }}</code>
          </span>
        </div>
        <div v-if="impersonation" class="notification is-warning impersonation-banner">
          <strong>Support impersonation active.</strong>
          Acting as user {{ impersonation.targetUserId }} until {{ impersonation.expiresAt }}.
          <b-button size="is-small" type="is-danger" outlined @click="stopImpersonation">End session</b-button>
        </div>
        <div class="global-notices" v-if="isGlobalNotices">
          <div v-if="serverConfig.needs_restart" class="notification is-danger">
            {{ $t('settings.needsRestart') }}
            &mdash;
            <b-button class="is-primary" size="is-small"
              @click="$utils.confirm($t('settings.confirmRestart'), reloadApp)">
              {{ $t('settings.restart') }}
            </b-button>
          </div>

          <template v-if="serverConfig.update">
            <div v-if="serverConfig.update.update.is_new" class="notification is-success">
              {{ $t('settings.updateAvailable', {
                version: `${serverConfig.update.update.release_version}
              (${$utils.getDate(serverConfig.update.update.release_date).format('DD MMM YY')})`,
              }) }}
              <a :href="serverConfig.update.update.url" target="_blank" rel="noopener noreferer">View</a>
            </div>

            <template v-if="serverConfig.update.messages && serverConfig.update.messages.length > 0">
              <div v-for="m in serverConfig.update.messages" class="notification"
                :class="{ [m.priority === 'high' ? 'is-danger' : 'is-info']: true }" :key="m.title">
                <h3 class="is-size-5" v-if="m.title"><strong>{{ m.title }}</strong></h3>
                <p v-if="m.description">{{ m.description }}</p>
                <a v-if="m.url" :href="m.url" target="_blank" rel="noopener noreferer">View</a>
              </div>
            </template>
          </template>

          <div v-if="serverConfig.has_legacy_user" class="notification is-danger">
            <b-icon icon="warning-empty" />
            Remove the <code>admin_username</code> and <code>admin_password</code> fields from the TOML
            configuration file or environment variables. If you are using APIs, create and use new API credentials
            before removing them. Visit
            <router-link :to="{ name: 'users' }">
              Admin -> Settings -> Users
            </router-link> dashboard. <a href="https://listmonk.app/docs/upgrade/#upgrading-to-v4xx" target="_blank"
              rel="noopener noreferer">Learn more.</a>
          </div>
        </div>

        <router-view :key="$route.fullPath" />
      </div>
    </div>

    <b-loading v-if="!$root.isLoaded" active />
  </div>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import { uris } from './constants';

import Navigation from './components/Navigation.vue';

export default Vue.extend({
  name: 'App',

  components: {
    Navigation,
  },

  data() {
    return {
      activeItem: {},
      activeGroup: {},
      windowWidth: window.innerWidth,
      impersonation: null,
    };
  },

  watch: {
    $route(to) {
      // Set the current route name to true for active+expanded keys in the
      // menu to pick up.
      this.activeItem = { [to.name]: true };
      if (to.meta.group) {
        this.activeGroup = { [to.meta.group]: true };
      } else {
        // Reset activeGroup to collapse menu items on navigating
        // to non group items from sidebar
        this.activeGroup = {};
      }
    },
  },

  methods: {
    loadImpersonation() {
      const raw = window.localStorage.getItem('mailview.impersonation');
      if (!raw) {
        this.impersonation = null;
        return;
      }
      try {
        const value = JSON.parse(raw);
        if (!value.id || new Date(value.expiresAt) <= new Date()) {
          window.localStorage.removeItem('mailview.impersonation');
          this.impersonation = null;
          return;
        }
        this.impersonation = value;
      } catch (e) {
        window.localStorage.removeItem('mailview.impersonation');
        this.impersonation = null;
      }
    },

    stopImpersonation() {
      window.localStorage.removeItem('mailview.impersonation');
      this.impersonation = null;
      window.location.reload();
    },

    toggleGroup(group, state) {
      this.activeGroup = state ? { [group]: true } : {};
    },

    emitPageRefresh() {
      this.$root.$emit('page.refresh');
    },

    reloadApp() {
      this.$api.reloadApp().then(() => {
        this.$utils.toast('Reloading app ...');

        // Poll until there's a 200 response, waiting for the app
        // to restart and come back up.
        const pollId = setInterval(() => {
          this.$api.getHealth().then(() => {
            clearInterval(pollId);
            document.location.reload();
          });
        }, 500);
      });
    },

    doLogout() {
      this.$api.logout().then(() => {
        document.location.href = uris.root;
      });
    },

    listenEvents() {
      const reMatchLog = /(.+?)\.go:\d+:(.+?)$/im;
      const evtSource = new EventSource(uris.errorEvents, { withCredentials: true });
      let numEv = 0;
      evtSource.onmessage = (e) => {
        if (numEv > 50) {
          return;
        }
        numEv += 1;

        const d = JSON.parse(e.data);
        if (d && d.type === 'error') {
          const msg = reMatchLog.exec(d.message.trim());
          this.$utils.toast(msg[2], 'is-danger', null, true);
        }
      };
    },
  },

  computed: {
    ...mapState(['serverConfig', 'profile']),

    tenantHostname() {
      return window.location.hostname;
    },

    shortActiveTenantId() {
      const id = this.profile.mailviewTenantId || '';
      return id ? `${id.slice(0, 8)}…${id.slice(-4)}` : '';
    },

    isGlobalNotices() {
      return (this.serverConfig.needs_restart
        || this.serverConfig.has_legacy_user
        || (this.serverConfig.update
          && this.serverConfig.update.messages
          && this.serverConfig.update.messages.length > 0));
    },

    version() {
      return import.meta.env.VUE_APP_VERSION;
    },

    isMobile() {
      return this.windowWidth <= 768;
    },
  },

  mounted() {
    this.loadImpersonation();
    window.addEventListener('storage', this.loadImpersonation);
    // Lists is required across different views. On app load, fetch the lists
    // and have them in the store.
    this.$api.getLists({ minimal: true, per_page: 'all', status: 'active' });

    window.addEventListener('resize', () => {
      this.windowWidth = window.innerWidth;
    });

    this.listenEvents();
  },

  beforeDestroy() {
    window.removeEventListener('storage', this.loadImpersonation);
  },
});
</script>

<style lang="scss">
@import "assets/style.scss";
@import "assets/icons/fontello.css";

.tenant-context-banner {
  display: flex;
  align-items: center;
  gap: .8rem;
  margin: -1rem -1rem 1.25rem;
  padding: .7rem 1rem;
  border-bottom: 1px solid #bcd2ef;
  color: #203a5d;
  background: linear-gradient(90deg, #eef5ff 0%, #f7faff 100%);
}

.tenant-context-banner__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 7px;
  color: #fff;
  background: #0055d4;
}

.tenant-context-banner__copy small,
.tenant-context-banner__copy strong {
  display: block;
}

.tenant-context-banner__copy small {
  color: #557196;
  font-size: .62rem;
  font-weight: 600;
  letter-spacing: .12em;
}

.tenant-context-banner__copy strong {
  font-size: .82rem;
}

.tenant-context-banner__boundary {
  display: flex;
  align-items: center;
  gap: .55rem;
  margin-left: auto;
  padding: .35rem .55rem;
  border: 1px solid #bdd0e9;
  border-radius: 6px;
  color: #345271;
  background: rgba(255, 255, 255, .7);
  font-size: .68rem;
}

.tenant-context-banner__boundary code {
  color: #486580;
  background: transparent;
  font-size: .65rem;
}

@media screen and (max-width: 600px) {
  .tenant-context-banner__boundary span { display: none; }
}
</style>

import { ToastProgrammatic as Toast } from 'buefy';
import axios from 'axios';
import qs from 'qs';
import store from '../store';
import { models } from '../constants';
import Utils from '../utils';

const http = axios.create({
  baseURL: import.meta.env.VUE_APP_ROOT_URL || '/',
  withCredentials: false,
  responseType: 'json',

  // Override the default serializer to switch params from becoming []id=a&[]id=b ...
  // in GET and DELETE requests to id=a&id=b.
  paramsSerializer: (params) => qs.stringify(params, { arrayFormat: 'repeat' }),
});

const utils = new Utils();

// Intercept requests to set the 'loading' state of a model.
http.interceptors.request.use((config) => {
  if ('loading' in config) {
    store.commit('setLoading', { model: config.loading, status: true });
  }
  return config;
}, (error) => Promise.reject(error));

// Intercept responses to set them to store.
http.interceptors.response.use((resp) => {
  // Clear the loading state for a model.
  if ('loading' in resp.config) {
    store.commit('setLoading', { model: resp.config.loading, status: false });
  }

  let data = {};
  if (typeof resp.data.data === 'object') {
    if (resp.data.data.constructor === Object) {
      data = { ...resp.data.data };
    } else {
      data = [...resp.data.data];
    }

    // Transform keys to camelCase.
    switch (typeof resp.config.camelCase) {
      case 'function':
        data = utils.camelKeys(data, resp.config.camelCase);
        break;
      case 'boolean':
        if (resp.config.camelCase) {
          data = utils.camelKeys(data);
        }
        break;
      default:
        data = utils.camelKeys(data);
        break;
    }
  } else {
    data = resp.data.data;
  }

  // Store the API response for a model.
  if ('store' in resp.config) {
    store.commit('setModelResponse', { model: resp.config.store, data });
  }

  return data;
}, (err) => {
  // Clear the loading state for a model.
  if ('loading' in err.config) {
    store.commit('setLoading', { model: err.config.loading, status: false });
  }

  let msg = '';
  if (err.response && err.response.data && err.response.data.message) {
    msg = err.response.data.message;
  } else {
    msg = err.toString();
  }

  if (!err.config.disableToast) {
    Toast.open({
      message: msg,
      type: 'is-danger',
      queue: false,
      position: 'is-top',
      pauseOnHover: true,
    });
  }

  return Promise.reject(err);
});

// API calls accept the following config keys.
// loading: modelName (set's the loading status in the global store: eg: store.loading.lists = true)
// store: modelName (set's the API response in the global store. eg: store.lists: { ... } )

// Health check endpoint that does not throw a toast.
export const getHealth = () => http.get(
  '/api/health',
  { disableToast: true },
);

export const reloadApp = () => http.post('/api/admin/reload');

// Dashboard
export const getDashboardCounts = () => http.get(
  '/api/dashboard/counts',
  { loading: models.dashboard },
);

export const getDashboardCharts = () => http.get(
  '/api/dashboard/charts',
  { loading: models.dashboard },
);

// Lists.
export const getLists = (params) => http.get(
  '/api/lists',
  {
    params: (!params ? { per_page: 'all' } : params),
    loading: models.lists,
    store: models.lists,
  },
);

export const queryLists = (params) => http.get(
  '/api/lists',
  {
    params: (!params ? { per_page: 'all' } : params),
    loading: models.listsFull,
  },
);

export const getList = async (id) => http.get(
  `/api/lists/${id}`,
  { loading: models.list },
);

export const createList = (data) => http.post(
  '/api/lists',
  data,
  { loading: models.lists },
);

export const updateList = (data) => http.put(
  `/api/lists/${data.id}`,
  data,
  { loading: models.lists },
);

export const deleteList = (id) => http.delete(
  `/api/lists/${id}`,
  { loading: models.lists },
);

export const deleteLists = (params) => http.delete(
  '/api/lists',
  { params, loading: models.lists },
);

// Subscribers.
export const getSubscribers = async (params) => http.get(
  '/api/subscribers',
  {
    params,
    loading: models.subscribers,
    store: models.subscribers,
    camelCase: (keyPath) => !keyPath.startsWith('.results.*.attribs'),
  },
);

export const getSubscriber = async (id) => http.get(
  `/api/subscribers/${id}`,
  { loading: models.subscribers },
);

export const getSubscriberActivity = async (id) => http.get(
  `/api/subscribers/${id}/activity`,
  { loading: models.subscribers },
);

export const getSubscriberBounces = async (id) => http.get(
  `/api/subscribers/${id}/bounces`,
  { loading: models.bounces },
);

export const deleteSubscriberBounces = async (id) => http.delete(
  `/api/subscribers/${id}/bounces`,
  { loading: models.bounces },
);

export const deleteBounce = async (id) => http.delete(
  `/api/bounces/${id}`,
  { loading: models.bounces },
);

export const deleteBounces = async (params) => http.delete(
  '/api/bounces',
  { params, loading: models.bounces },
);

export const blocklistBouncedSubscribers = async () => http.put(
  '/api/bounces/blocklist',
  { loading: models.bounces },
);

export const createSubscriber = (data) => http.post(
  '/api/subscribers',
  data,
  { loading: models.subscribers },
);

export const updateSubscriber = (data) => http.put(
  `/api/subscribers/${data.id}`,
  data,
  { loading: models.subscribers },
);

export const sendSubscriberOptin = (id) => http.post(
  `/api/subscribers/${id}/optin`,
  {},
  { loading: models.subscribers },
);

export const deleteSubscriber = (id) => http.delete(
  `/api/subscribers/${id}`,
  { loading: models.subscribers },
);

export const addSubscribersToLists = (data) => http.put(
  '/api/subscribers/lists',
  data,
  { loading: models.subscribers },
);

export const addSubscribersToListsByQuery = (data) => http.put(
  '/api/subscribers/query/lists',
  data,

  { loading: models.subscribers },
);

export const blocklistSubscribers = (data) => http.put(
  '/api/subscribers/blocklist',
  data,
  { loading: models.subscribers },
);

export const blocklistSubscribersByQuery = (data) => http.put(
  '/api/subscribers/query/blocklist',
  data,
  { loading: models.subscribers },
);

export const deleteSubscribers = (params) => http.delete(
  '/api/subscribers',
  { params, loading: models.subscribers },
);

export const deleteSubscribersByQuery = (data) => http.post(
  '/api/subscribers/query/delete',
  data,
  { loading: models.subscribers },
);

// Subscriber import. Tenant hosts transparently use MailView's isolated CSV
// jobs while the legacy workspace keeps the upstream ZIP importer contract.
let dismissedTenantImportID = '';
const isTenantHost = () => Boolean(store.state.profile && store.state.profile.mailviewTenantId);
const tenantImportStatus = (job) => {
  if (!job || job.id === dismissedTenantImportID) return { status: 'none' };
  const statuses = {
    pending: 'importing',
    processing: 'importing',
    completed: 'finished',
    cancelled: 'stopped',
    failed: 'failed',
  };
  return {
    id: job.id,
    status: statuses[job.status] || 'failed',
    imported: job.importedRows || 0,
    total: job.totalRows || 0,
    error: job.error || '',
  };
};
const getLatestTenantImport = async () => {
  const jobs = await http.get('/api/mailview/data/import-jobs');
  return jobs.length > 0 ? jobs[0] : null;
};

export const importSubscribers = (data) => {
  if (!isTenantHost()) return http.post('/api/import/subscribers', data);
  const params = JSON.parse(data.get('params') || '{}');
  const payload = new FormData();
  payload.set('file', data.get('file'));
  payload.set('list_ids', (params.lists || []).join(','));
  payload.set('idempotency_key', `${Date.now()}-${Math.random().toString(36).slice(2)}`);
  dismissedTenantImportID = '';
  return http.post('/api/mailview/data/import-jobs', payload);
};

export const getImportStatus = async () => {
  if (!isTenantHost()) return http.get('/api/import/subscribers');
  return tenantImportStatus(await getLatestTenantImport());
};

export const getImportLogs = async () => {
  if (!isTenantHost()) {
    return http.get('/api/import/subscribers/logs', { camelCase: false });
  }
  const job = await getLatestTenantImport();
  if (!job || job.id === dismissedTenantImportID) return '';
  const progress = `${job.processedRows || 0}/${job.totalRows || 0} rows processed`;
  return job.error ? `${progress}\n${job.error}` : progress;
};

export const stopImport = async () => {
  if (!isTenantHost()) return http.delete('/api/import/subscribers');
  const job = await getLatestTenantImport();
  if (!job) return { status: 'none' };
  if (job.status === 'pending' || job.status === 'processing') {
    await http.post(`/api/mailview/data/import-jobs/${job.id}/cancel`);
  }
  dismissedTenantImportID = job.id;
  return { status: 'none' };
};

// Bounces.
export const getBounces = async (params) => http.get(
  '/api/bounces',
  { params, loading: models.bounces },
);

// Campaigns.
export const getCampaigns = async (params) => http.get('/api/campaigns', {
  params,
  loading: models.campaigns,
  store: models.campaigns,
  camelCase: (keyPath) => !keyPath.startsWith('.results.*.headers'),
});

export const getCampaign = async (id) => http.get(`/api/campaigns/${id}`, {
  loading: models.campaigns,
  camelCase: (keyPath) => !keyPath.startsWith('.headers'),
});

export const getCampaignStats = async () => http.get('/api/campaigns/running/stats', {});

export const createCampaign = async (data) => http.post(
  '/api/campaigns',
  data,
  { loading: models.campaigns },
);

export const getCampaignViewCounts = async (params) => http.get(
  '/api/campaigns/analytics/views',
  { params, loading: models.campaigns },
);

export const getCampaignClickCounts = async (params) => http.get(
  '/api/campaigns/analytics/clicks',
  { params, loading: models.campaigns },
);

export const getCampaignBounceCounts = async (params) => http.get(
  '/api/campaigns/analytics/bounces',
  { params, loading: models.campaigns },
);

export const getCampaignLinkCounts = async (params) => http.get(
  '/api/campaigns/analytics/links',
  { params, loading: models.campaigns },
);

export const convertCampaignContent = async (data) => http.post(
  `/api/campaigns/${data.id}/content`,
  data,
  { loading: models.campaigns },
);

export const testCampaign = async (data) => http.post(
  `/api/campaigns/${data.id}/test`,
  data,
  { loading: models.campaigns },
);

export const updateCampaign = async (id, data) => http.put(
  `/api/campaigns/${id}`,
  data,
  { loading: models.campaigns },
);

export const changeCampaignStatus = async (id, status) => http.put(
  `/api/campaigns/${id}/status`,
  { status },

  { loading: models.campaigns },
);

export const updateCampaignArchive = async (id, data) => http.put(
  `/api/campaigns/${id}/archive`,
  data,
  { loading: models.campaigns },
);

export const deleteCampaign = async (id) => http.delete(
  `/api/campaigns/${id}`,
  { loading: models.campaigns },
);

export const deleteCampaigns = (params) => http.delete(
  '/api/campaigns',
  { params, loading: models.campaigns },
);

// Media.
export const getMedia = async (params) => http.get(
  '/api/media',
  { params, loading: models.media, store: models.media },
);

export const uploadMedia = (data) => http.post(
  '/api/media',
  data,
  { loading: models.media },
);

export const deleteMedia = (id) => http.delete(
  `/api/media/${id}`,
  { loading: models.media },
);

// Templates.
export const createTemplate = async (data) => http.post(
  '/api/templates',
  data,
  { loading: models.templates },
);

export const getTemplates = async () => http.get(
  '/api/templates',
  { loading: models.templates, store: models.templates },
);

export const getTemplate = async (id) => http.get(
  `/api/templates/${id}`,
  { loading: models.templates },
);

export const updateTemplate = async (data) => http.put(
  `/api/templates/${data.id}`,
  data,
  { loading: models.templates },
);

export const makeTemplateDefault = async (id) => http.put(
  `/api/templates/${id}/default`,
  {},
  { loading: models.templates },
);

export const deleteTemplate = async (id) => http.delete(
  `/api/templates/${id}`,
  { loading: models.templates },
);

// Settings.
export const getServerConfig = async () => http.get(
  '/api/config',
  { loading: models.serverConfig, store: models.serverConfig, camelCase: false },
);

export const getSettings = async () => http.get(
  '/api/settings',
  { loading: models.settings, store: models.settings, camelCase: false },
);

export const updateSettings = async (data) => http.put(
  '/api/settings',
  data,
  { loading: models.settings },
);

export const updateSettingsByKey = async (key, data) => http.put(
  `/api/settings/${key}`,
  data,
  { loading: models.settings },
);

export const testSMTP = async (data) => http.post(
  '/api/settings/smtp/test',
  data,
  { loading: models.settings, disableToast: true },
);

export const getLogs = async () => http.get(
  '/api/logs',
  { loading: models.logs, camelCase: false },
);

export const getLang = async (lang) => http.get(
  `/api/lang/${lang}`,
  { loading: models.lang, camelCase: false },
);

export const logout = async () => http.post('/api/logout');

export const deleteGCCampaignAnalytics = async (typ, beforeDate) => http.delete(
  `/api/maintenance/analytics/${typ}`,
  { loading: models.maintenance, params: { before_date: beforeDate } },
);

export const deleteGCSubscribers = async (typ) => http.delete(
  `/api/maintenance/subscribers/${typ}`,
  { loading: models.maintenance },
);

export const deleteGCSubscriptions = async (beforeDate) => http.delete(
  '/api/maintenance/subscriptions/unconfirmed',
  { loading: models.maintenance, params: { before_date: beforeDate } },
);

// Users.
export const getUsers = () => http.get(
  '/api/users',
  {
    loading: models.users,
    store: models.users,
  },
);

export const queryUsers = () => http.get(
  '/api/users',
  {
    loading: models.users,
    store: models.users,
  },
);

export const getUser = async (id) => http.get(
  `/api/users/${id}`,
  { loading: models.users },
);

export const createUser = (data) => http.post(
  '/api/users',
  data,
  { loading: models.users },
);

export const updateUser = (data) => http.put(
  `/api/users/${data.id}`,
  data,
  { loading: models.users },
);

export const deleteUser = (id) => http.delete(
  `/api/users/${id}`,
  { loading: models.users },
);

export const getUserProfile = () => http.get(
  '/api/profile',
  { loading: models.users, store: models.profile },
);

export const updateUserProfile = (data) => http.put(
  '/api/profile',
  data,
  { loading: models.users, store: models.profile },
);

export const getUserRoles = async () => http.get(
  '/api/roles/users',
  { loading: models.userRoles, store: models.userRoles },
);

export const getListRoles = async () => http.get(
  '/api/roles/lists',
  { loading: models.listRoles, store: models.listRoles },
);

export const createUserRole = (data) => http.post(
  '/api/roles/users',
  data,
  { loading: models.userRoles },
);

export const createListRole = (data) => http.post(
  '/api/roles/lists',
  data,
  { loading: models.listRoles },
);

export const updateUserRole = (data) => http.put(
  `/api/roles/users/${data.id}`,
  data,
  { loading: models.userRoles },
);

export const updateListRole = (data) => http.put(
  `/api/roles/lists/${data.id}`,
  data,
  { loading: models.userRoles },
);

export const deleteRole = (id) => http.delete(
  `/api/roles/${id}`,
  { loading: models.userRoles },
);

// TOTP 2FA APIs
export const getTOTPQR = (id) => http.get(
  `/api/users/${id}/twofa/totp`,
  { camelCase: true },
);

export const enableTOTP = (id, data) => http.put(
  `/api/users/${id}/twofa`,
  data,
);

export const disableTOTP = (id, data) => http.delete(
  `/api/users/${id}/twofa`,
  { data },
);

// MailView Control Plane. The backend and UI gate each operation using the
// effective platform permissions, with the legacy Super Admin bridge.
export const getMailViewTenants = () => http.get(
  '/api/mailview/tenants',
  { loading: models.mailviewTenants },
);

export const createMailViewTenant = (data) => http.post(
  '/api/mailview/tenants',
  data,
  { loading: models.mailviewTenants },
);

export const updateMailViewTenantStatus = (id, status) => http.patch(
  `/api/mailview/tenants/${id}`,
  { status },
  { loading: models.mailviewTenants },
);

export const getMailViewTenantDomains = (tenantID) => http.get(
  `/api/mailview/tenants/${tenantID}/domains`,
  { loading: models.mailviewTenants },
);

export const createMailViewTenantDomain = (tenantID, data) => http.post(
  `/api/mailview/tenants/${tenantID}/domains`,
  data,
  { loading: models.mailviewTenants },
);

export const verifyMailViewTenantDomain = (tenantID, domainID) => http.post(
  `/api/mailview/tenants/${tenantID}/domains/${domainID}/verify`,
  {},
  { loading: models.mailviewTenants },
);

export const revokeMailViewTenantDomain = (tenantID, domainID) => http.post(
  `/api/mailview/tenants/${tenantID}/domains/${domainID}/revoke`,
  {},
  { loading: models.mailviewTenants },
);

export const getMailViewTenantQuota = (tenantID) => http.get(
  `/api/mailview/tenants/${tenantID}/quota`,
  { loading: models.mailviewTenants },
);

export const setMailViewTenantQuotaPlan = (tenantID, planCode) => http.put(
  `/api/mailview/tenants/${tenantID}/quota`,
  { plan_code: planCode },
  { loading: models.mailviewTenants },
);

export const getMailViewTenantPlans = () => http.get(
  '/api/mailview/plans',
  { loading: models.mailviewTenants },
);

export const getMailViewPlatformRoles = () => http.get(
  '/api/mailview/platform/roles',
  { loading: models.mailviewTenants },
);

export const getMailViewPlatformAssignments = () => http.get(
  '/api/mailview/platform/assignments',
  { loading: models.mailviewTenants },
);

export const assignMailViewPlatformRole = (userID, roleID) => http.post(
  '/api/mailview/platform/assignments',
  { user_id: userID, role_id: roleID },
  { loading: models.mailviewTenants },
);

export const revokeMailViewPlatformRole = (userID, roleID) => http.delete(
  `/api/mailview/platform/assignments/${userID}/${roleID}`,
  { loading: models.mailviewTenants },
);

export const getMailViewDashboard = () => http.get(
  '/api/mailview/dashboard',
  { loading: models.mailviewTenants },
);

export const resetMailViewTenantOwner = (tenantID, newOwnerUserID) => http.post(
  `/api/mailview/tenants/${tenantID}/owner`,
  { new_owner_user_id: newOwnerUserID },
  { loading: models.mailviewTenants },
);

export const setMailViewTenantInfrastructure = (tenantID, mode) => http.post(
  `/api/mailview/tenants/${tenantID}/infrastructure`,
  { mode },
  { loading: models.mailviewTenants },
);

export const startMailViewImpersonation = (data) => http.post(
  '/api/mailview/platform/impersonation',
  data,
  { loading: models.mailviewTenants },
);

export const listMailViewImpersonationGrants = () => http.get(
  '/api/mailview/platform/impersonation',
  { loading: models.mailviewTenants },
);

export const revokeMailViewImpersonation = (grantID) => http.post(
  `/api/mailview/platform/impersonation/${grantID}/revoke`,
  {},
  { loading: models.mailviewTenants },
);

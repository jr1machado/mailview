<template>
  <section class="mailview-tenants">
    <header v-if="canManageTenants" class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">
          MailView Tenants
          <span v-if="!isNaN(tenants.length)">({{ tenants.length }})</span>
        </h1>
      </div>
      <div class="column has-text-right">
        <b-field expanded>
          <b-button expanded type="is-primary" icon-left="plus" @click="showNewTenantForm">
            {{ $t('globals.buttons.new') }}
          </b-button>
        </b-field>
      </div>
    </header>

    <div class="columns dashboard-cards" v-if="canManageTenants && dashboard">
      <div class="column"><div class="box"><p class="heading">Active tenants</p><p class="title">{{ dashboard.tenantsActive }}</p></div></div>
      <div class="column"><div class="box"><p class="heading">Suspended</p><p class="title">{{ dashboard.tenantsSuspended }}</p></div></div>
      <div class="column"><div class="box"><p class="heading">Domains pending verification</p><p class="title">{{ dashboard.domainsPendingVerification }}</p></div></div>
      <div class="column"><div class="box"><p class="heading">Audit events (24h)</p><p class="title">{{ dashboard.auditEventsLast24h }}</p></div></div>
      <div class="column"><div class="box"><p class="heading">Active impersonation grants</p><p class="title">{{ dashboard.activeImpersonationGrants }}</p></div></div>
      <div class="column"><div class="box"><p class="heading">Bounces (24h)</p><p class="title">{{ dashboard.bouncesLast24h }}</p></div></div>
      <div class="column"><div class="box"><p class="heading">Webhook failures</p><p class="title">{{ dashboard.webhookFailuresLast24h }}</p></div></div>
      <div class="column"><div class="box"><p class="heading">Open incidents</p><p class="title">{{ dashboard.openIncidents }}</p></div></div>
    </div>

    <b-table v-if="canManageTenants" :data="tenants" :loading="loading.mailviewTenants" hoverable>
      <b-table-column v-slot="props" field="slug" label="Slug" sortable>
        <a @click.prevent="selectTenant(props.row)" @keyup.enter.prevent="selectTenant(props.row)"
          tabindex="0">{{ props.row.slug }}</a>
      </b-table-column>
      <b-table-column v-slot="props" field="name" label="Name" sortable>
        {{ props.row.name }}
      </b-table-column>
      <b-table-column v-slot="props" field="status" label="Status" sortable>
        <b-tag :type="statusTagType(props.row.status)">{{ props.row.status }}</b-tag>
      </b-table-column>
      <b-table-column v-slot="props" label="Actions">
        <div class="buttons">
          <b-button size="is-small" @click="selectTenant(props.row)">Domains / quota</b-button>
          <b-button v-if="props.row.status === 'active'" size="is-small" type="is-warning"
            @click="setStatus(props.row, 'suspended')">
Suspend
</b-button>
          <b-button v-else-if="props.row.status === 'suspended'" size="is-small" type="is-success"
            @click="setStatus(props.row, 'active')">
Reactivate
</b-button>
        </div>
      </b-table-column>
    </b-table>

    <hr v-if="canManagePlatformRoles" />

    <template v-if="canManagePlatformRoles">
    <h2 class="title is-5">Platform roles</h2>
    <p class="has-text-grey">
Grants tenant.manage.platform / platform.roles.manage etc. Independent from tenant
      membership roles.
</p>
    <b-table :data="assignments" :loading="loading.mailviewTenants" hoverable>
      <b-table-column v-slot="props" field="userId" label="User ID">{{ props.row.userId }}</b-table-column>
      <b-table-column v-slot="props" field="roleName" label="Role">{{ props.row.roleName }}</b-table-column>
      <b-table-column v-slot="props" label="Actions">
        <b-button size="is-small" type="is-danger" @click="revokeAssignment(props.row)">Revoke</b-button>
      </b-table-column>
    </b-table>
    <b-field grouped class="assign-form">
      <b-input v-model.number="assignForm.userId" placeholder="User ID" type="number" />
      <b-select v-model="assignForm.roleId" placeholder="Platform role">
        <option v-for="r in platformRoles" :key="r.id" :value="r.id">{{ r.name }}</option>
      </b-select>
      <b-button type="is-primary" @click="assignRole">Assign</b-button>
    </b-field>
    </template>

    <hr v-if="canImpersonate" />

    <template v-if="canImpersonate">
    <h2 class="title is-5">Impersonation grants</h2>
    <p class="has-text-grey">
Time-boxed, reason-required access to a tenant member's data. Requires the actor to have
      verified TOTP in the last 15 minutes. Never grants platform or billing access.
</p>
    <b-table :data="impersonationGrants" :loading="loading.mailviewTenants" hoverable>
      <b-table-column v-slot="props" field="targetUserId" label="Target user">{{ props.row.targetUserId }}</b-table-column>
      <b-table-column v-slot="props" field="reason" label="Reason">{{ props.row.reason }}</b-table-column>
      <b-table-column v-slot="props" field="expiresAt" label="Expires">{{ props.row.expiresAt }}</b-table-column>
      <b-table-column v-slot="props" label="Status">
        <b-tag :type="impersonationStatusTagType(props.row)">{{ impersonationStatus(props.row) }}</b-tag>
      </b-table-column>
      <b-table-column v-slot="props" label="Actions">
        <b-button v-if="props.row.approvalRequired && !props.row.approvedAt && props.row.actorUserId !== profile.id"
          size="is-small" type="is-success" @click="approveImpersonation(props.row)">
Approve
</b-button>
        <b-button v-if="impersonationStatus(props.row) === 'active'" size="is-small"
          @click="activateImpersonation(props.row)">
Use grant
</b-button>
        <b-button v-if="impersonationStatus(props.row) === 'active'" size="is-small" type="is-danger"
          @click="revokeImpersonation(props.row)">
Revoke
</b-button>
      </b-table-column>
    </b-table>
    <b-field grouped class="impersonation-form">
      <b-select v-if="canManageTenants" v-model="impersonationForm.tenantId" placeholder="Tenant">
        <option v-for="t in tenants" :key="t.id" :value="t.id">{{ t.slug }}</option>
      </b-select>
      <b-input v-else v-model="impersonationForm.tenantId" placeholder="Tenant UUID" />
      <b-input v-model.number="impersonationForm.targetUserId" placeholder="Target user ID" type="number" />
      <b-input v-model.number="impersonationForm.ttlMinutes" placeholder="TTL (min, max 30)" type="number" />
      <b-checkbox v-model="impersonationForm.requireApproval">Require approval</b-checkbox>
    </b-field>
    <b-field class="impersonation-form">
      <b-input v-model="impersonationForm.reason" placeholder="Reason (min 10 chars, required)" expanded />
      <b-button type="is-primary" @click="startImpersonation">Start</b-button>
    </b-field>
    </template>

    <!-- New tenant modal -->
    <b-modal v-if="canManageTenants" :active.sync="isNewTenantOpen" :width="500">
      <div class="box">
        <h3 class="title is-5">New tenant</h3>
        <b-field label="Slug"><b-input v-model="newTenant.slug" placeholder="acme-email" /></b-field>
        <b-field label="Name"><b-input v-model="newTenant.name" placeholder="Acme Email" /></b-field>
        <b-field label="Owner user ID"><b-input v-model.number="newTenant.ownerUserId" type="number" /></b-field>
        <b-button type="is-primary" expanded @click="createTenant">Create</b-button>
      </div>
    </b-modal>

    <!-- Tenant detail modal: domains + quota -->
    <b-modal v-if="canManageTenants" :active.sync="isDetailOpen" :width="700">
      <div class="box" v-if="selected">
        <h3 class="title is-5">{{ selected.slug }}</h3>

        <h4 class="title is-6">Tenant slug</h4>
        <p class="has-text-grey">Changes create a controlled HTTP 308 alias for the selected period.</p>
        <b-field grouped>
          <b-input v-model="slugForm.slug" placeholder="new-tenant-slug" />
          <b-input v-model.number="slugForm.redirectDays" type="number" min="1" max="365" />
          <b-button @click="changeSlug">Change slug</b-button>
        </b-field>

        <h4 class="title is-6">Quota</h4>
        <b-field grouped v-if="quota">
          <p>Plan: <strong>{{ quota.planCode }}</strong></p>
        </b-field>
        <b-field grouped>
          <b-select v-model="quotaPlanCode" placeholder="Plan">
            <option v-for="p in plans" :key="p.code" :value="p.code">{{ p.name }}</option>
          </b-select>
          <b-button @click="applyQuotaPlan">Apply plan</b-button>
        </b-field>

        <h4 class="title is-6">Domains</h4>
        <b-button size="is-small" @click="revalidateDomains">Revalidate due domains</b-button>
        <b-table :data="domains" hoverable>
          <b-table-column v-slot="props" field="hostname" label="Hostname">{{ props.row.hostname }}</b-table-column>
          <b-table-column v-slot="props" field="purpose" label="Purpose">{{ props.row.purpose }}</b-table-column>
          <b-table-column v-slot="props" field="status" label="Status">
            <b-tag :type="domainStatusTagType(props.row.status)">{{ props.row.status }}</b-tag>
          </b-table-column>
      <b-table-column v-slot="props" field="verificationName" label="DNS record">
      <code>{{ props.row.verificationName }}</code><br />
      <small>{{ props.row.verificationValue }}</small>
      </b-table-column>
          <b-table-column v-slot="props" label="Actions">
            <b-button v-if="props.row.status === 'pending' || props.row.status === 'failed'" size="is-small" type="is-success"
              @click="verifyDomain(props.row)">
Verify DNS
</b-button>
            <b-button v-if="props.row.status !== 'revoked'" size="is-small" type="is-danger"
              @click="revokeDomain(props.row)">
Revoke
</b-button>
          </b-table-column>
        </b-table>

        <b-field grouped class="new-domain-form">
          <b-input v-model="newDomain.hostname" placeholder="mail.cliente.com.br" />
          <b-select v-model="newDomain.purpose" placeholder="Purpose">
            <option value="portal">portal</option>
            <option value="tracking">tracking</option>
            <option value="sending">sending</option>
            <option value="return_path">return_path</option>
            <option value="public_forms">public_forms</option>
          </b-select>
      <b-select v-model="newDomain.verificationMethod" placeholder="DNS method">
      <option value="txt">TXT</option>
      <option value="cname">CNAME</option>
      </b-select>
          <b-button type="is-primary" @click="createDomain">Add domain</b-button>
        </b-field>

        <h4 class="title is-6">Owner</h4>
        <b-field grouped>
          <b-input v-model.number="resetOwnerUserId" placeholder="New owner user ID" type="number" />
          <b-button @click="resetOwner">Reset owner</b-button>
        </b-field>

        <h4 class="title is-6">Infrastructure (Enterprise)</h4>
        <p class="has-text-grey">
The Control Plane only accepts activation after all dedicated resource references are present.
</p>
        <b-field grouped>
      <b-select v-model="infrastructure.mode">
            <option value="shared">shared</option>
            <option value="dedicated_requested">dedicated_requested</option>
            <option value="dedicated">dedicated</option>
          </b-select>
          <b-button @click="applyInfrastructureMode">Apply</b-button>
        </b-field>
    <b-field v-if="infrastructure.mode !== 'shared'" grouped group-multiline>
      <b-input v-model="infrastructure.databaseRef" placeholder="Database secret ref" />
      <b-input v-model="infrastructure.workerRef" placeholder="Worker/queue ref" />
      <b-input v-model="infrastructure.smtpRef" placeholder="SMTP secret ref" />
      <b-input v-model="infrastructure.storageRef" placeholder="Storage ref" />
      <b-input v-model="infrastructure.encryptionKeyRef" placeholder="Encryption key ref" />
      <b-input v-model="infrastructure.dockerNamespace" placeholder="Docker namespace" />
    </b-field>
      </div>
    </b-modal>
  </section>
</template>

<script>
export default {
  name: 'MailViewTenants',

  data() {
    return {
      tenants: [],
      assignments: [],
      platformRoles: [],
      plans: [],
      isNewTenantOpen: false,
      isDetailOpen: false,
      newTenant: { slug: '', name: '', ownerUserId: null },
      assignForm: { userId: null, roleId: null },
      selected: null,
      domains: [],
      quota: null,
      quotaPlanCode: '',
      newDomain: { hostname: '', purpose: 'sending', verificationMethod: 'txt' },
      slugForm: { slug: '', redirectDays: 30 },
      dashboard: null,
      resetOwnerUserId: null,
      infrastructure: {
        mode: 'shared', databaseRef: '', workerRef: '', smtpRef: '', storageRef: '', encryptionKeyRef: '', dockerNamespace: '',
      },
      impersonationGrants: [],
      impersonationForm: {
        tenantId: null, targetUserId: null, ttlMinutes: 15, reason: '', requireApproval: false,
      },
    };
  },

  computed: {
    loading() {
      return this.$store.state.loading;
    },

    profile() {
      return this.$store.state.profile;
    },

    canManagePlatformRoles() {
      return this.$canPlatform('platform.roles.manage');
    },

    canManageTenants() {
      return this.$canPlatform('tenant.manage.platform');
    },

    canImpersonate() {
      return this.$canPlatform('support.impersonate.platform');
    },
  },

  methods: {
    statusTagType(status) {
      const types = {
        active: 'is-success', suspended: 'is-warning', pending: 'is-light', offboarded: 'is-danger',
      };
      return types[status] || 'is-light';
    },

    domainStatusTagType(status) {
      const types = {
        verified: 'is-success', pending: 'is-warning', failed: 'is-danger', revoked: 'is-light',
      };
      return types[status] || 'is-light';
    },

    async loadTenants() {
      this.tenants = await this.$api.getMailViewTenants();
    },

    async loadPlatformRoles() {
      this.platformRoles = await this.$api.getMailViewPlatformRoles();
    },

    async loadAssignments() {
      this.assignments = await this.$api.getMailViewPlatformAssignments();
    },

    async loadPlans() {
      this.plans = await this.$api.getMailViewTenantPlans();
    },

    showNewTenantForm() {
      this.newTenant = { slug: '', name: '', ownerUserId: null };
      this.isNewTenantOpen = true;
    },

    async createTenant() {
      await this.$api.createMailViewTenant({
        slug: this.newTenant.slug,
        name: this.newTenant.name,
        owner_user_id: this.newTenant.ownerUserId,
      });
      this.isNewTenantOpen = false;
      this.loadTenants();
    },

    async setStatus(tenant, status) {
      await this.$api.updateMailViewTenantStatus(tenant.id, status);
      this.loadTenants();
    },

    async assignRole() {
      if (!this.assignForm.userId || !this.assignForm.roleId) {
        return;
      }
      await this.$api.assignMailViewPlatformRole(this.assignForm.userId, this.assignForm.roleId);
      this.assignForm = { userId: null, roleId: null };
      this.loadAssignments();
    },

    async revokeAssignment(row) {
      await this.$api.revokeMailViewPlatformRole(row.userId, row.roleId);
      this.loadAssignments();
    },

    async selectTenant(tenant) {
      this.selected = tenant;
      this.isDetailOpen = true;
      this.resetOwnerUserId = null;
      this.slugForm = { slug: tenant.slug, redirectDays: 30 };
      this.domains = await this.$api.getMailViewTenantDomains(tenant.id);
      this.quota = await this.$api.getMailViewTenantQuota(tenant.id);
      this.quotaPlanCode = this.quota.planCode;
      this.infrastructure = await this.$api.getMailViewTenantInfrastructure(tenant.id);
    },

    async resetOwner() {
      if (!this.selected || !this.resetOwnerUserId) {
        return;
      }
      await this.$api.resetMailViewTenantOwner(this.selected.id, this.resetOwnerUserId);
      this.resetOwnerUserId = null;
    },

    async applyInfrastructureMode() {
      if (!this.selected) {
        return;
      }
      await this.$api.setMailViewTenantInfrastructure(this.selected.id, {
        mode: this.infrastructure.mode,
        database_ref: this.infrastructure.databaseRef || '',
        worker_ref: this.infrastructure.workerRef || '',
        smtp_ref: this.infrastructure.smtpRef || '',
        storage_ref: this.infrastructure.storageRef || '',
        encryption_key_ref: this.infrastructure.encryptionKeyRef || '',
        docker_namespace: this.infrastructure.dockerNamespace || '',
      });
      this.infrastructure = await this.$api.getMailViewTenantInfrastructure(this.selected.id);
    },

    async changeSlug() {
      if (!this.selected || !this.slugForm.slug) return;
      const result = await this.$api.changeMailViewTenantSlug(this.selected.id, {
        slug: this.slugForm.slug, redirect_days: this.slugForm.redirectDays,
      });
      this.selected = result.tenant;
      await this.loadTenants();
    },

    async loadDashboard() {
      this.dashboard = await this.$api.getMailViewDashboard();
    },

    async loadImpersonationGrants() {
      this.impersonationGrants = await this.$api.listMailViewImpersonationGrants();
    },

    impersonationStatus(grant) {
      if (grant.revokedAt) {
        return 'revoked';
      }
      if (grant.approvalRequired && !grant.approvedAt) {
        return 'pending approval';
      }
      return new Date(grant.expiresAt) > new Date() ? 'active' : 'expired';
    },

    impersonationStatusTagType(grant) {
      const types = {
        active: 'is-success', expired: 'is-light', revoked: 'is-danger', 'pending approval': 'is-warning',
      };
      return types[this.impersonationStatus(grant)];
    },

    async startImpersonation() {
      const {
        tenantId, targetUserId, ttlMinutes, reason,
      } = this.impersonationForm;
      if (!tenantId || !targetUserId || !reason || reason.trim().length < 10) {
        return;
      }
      await this.$api.startMailViewImpersonation({
        tenant_id: tenantId,
        target_user_id: targetUserId,
        ttl_minutes: ttlMinutes,
        reason,
        require_approval: this.impersonationForm.requireApproval,
      });
      this.impersonationForm = {
        tenantId: null, targetUserId: null, ttlMinutes: 15, reason: '', requireApproval: false,
      };
      this.loadImpersonationGrants();
      if (this.canManageTenants) this.loadDashboard();
    },

    async revokeImpersonation(grant) {
      await this.$api.revokeMailViewImpersonation(grant.id);
      this.loadImpersonationGrants();
      if (this.canManageTenants) this.loadDashboard();
    },

    async approveImpersonation(grant) {
      await this.$api.approveMailViewImpersonation(grant.id);
      this.loadImpersonationGrants();
    },

    activateImpersonation(grant) {
      window.localStorage.setItem('mailview.impersonation', JSON.stringify({
        id: grant.id,
        tenantId: grant.tenantId,
        targetUserId: grant.targetUserId,
        expiresAt: grant.expiresAt,
      }));
      window.dispatchEvent(new StorageEvent('storage', { key: 'mailview.impersonation' }));
      this.$utils.toast('Impersonation active. Open the tenant host to use this grant.', 'is-warning');
    },

    async applyQuotaPlan() {
      if (!this.selected || !this.quotaPlanCode) {
        return;
      }
      this.quota = await this.$api.setMailViewTenantQuotaPlan(this.selected.id, this.quotaPlanCode);
    },

    async createDomain() {
      if (!this.selected || !this.newDomain.hostname) {
        return;
      }
      await this.$api.createMailViewTenantDomain(this.selected.id, {
        hostname: this.newDomain.hostname,
        purpose: this.newDomain.purpose,
        verification_method: this.newDomain.verificationMethod,
      });
      this.newDomain = { hostname: '', purpose: 'sending', verificationMethod: 'txt' };
      this.domains = await this.$api.getMailViewTenantDomains(this.selected.id);
    },

    async verifyDomain(domain) {
      await this.$api.verifyMailViewTenantDomain(this.selected.id, domain.id);
      this.domains = await this.$api.getMailViewTenantDomains(this.selected.id);
    },

    async revokeDomain(domain) {
      await this.$api.revokeMailViewTenantDomain(this.selected.id, domain.id);
      this.domains = await this.$api.getMailViewTenantDomains(this.selected.id);
    },

    async revalidateDomains() {
      await this.$api.revalidateMailViewTenantDomains();
      this.domains = await this.$api.getMailViewTenantDomains(this.selected.id);
    },
  },

  mounted() {
    if (this.canManageTenants) {
      this.loadTenants();
      this.loadPlans();
      this.loadDashboard();
    }
    if (this.canManagePlatformRoles) {
      this.loadPlatformRoles();
      this.loadAssignments();
    }
    if (this.canImpersonate) {
      this.loadImpersonationGrants();
    }
  },
};
</script>

<style scoped>
.assign-form, .new-domain-form, .impersonation-form {
  margin-top: 1rem;
}
.dashboard-cards {
  margin-bottom: 1rem;
}
</style>

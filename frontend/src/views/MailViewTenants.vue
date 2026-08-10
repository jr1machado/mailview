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

    <section v-if="canManageTenants" class="environment-map" aria-labelledby="environment-map-title">
      <div class="environment-map__header">
        <div>
          <p class="environment-map__eyebrow">VISÃO DE ISOLAMENTO</p>
          <h2 id="environment-map-title" class="title is-5">Ambientes por cliente</h2>
          <p class="environment-map__description">
            Cada cartão representa um tenant e a faixa indica onde seus recursos são executados.
            Mesmo no ambiente compartilhado, dados e operações permanecem delimitados pelo tenant.
          </p>
        </div>
        <div class="environment-map__legend" aria-label="Legenda dos ambientes">
          <span><i class="environment-dot environment-dot--shared" /> Compartilhado</span>
          <span><i class="environment-dot environment-dot--requested" /> Em provisionamento</span>
          <span><i class="environment-dot environment-dot--dedicated" /> Dedicado</span>
        </div>
      </div>

      <div class="environment-flow" aria-hidden="true">
        <div class="environment-flow__platform">
          <b-icon icon="view-dashboard-variant-outline" size="is-small" />
          <span>MailView Control Plane</span>
        </div>
        <div class="environment-flow__line" />
        <div class="environment-flow__boundary">
          <b-icon icon="check-circle-outline" size="is-small" />
          <span>roteamento + limite de tenant</span>
        </div>
      </div>

      <div class="environment-lanes">
        <article v-for="group in environmentGroups" :key="group.mode"
          class="environment-lane" :class="`environment-lane--${group.mode}`">
          <header class="environment-lane__header">
            <div>
              <p class="environment-lane__title">{{ group.title }}</p>
              <p class="environment-lane__subtitle">{{ group.subtitle }}</p>
            </div>
            <span class="environment-lane__count">{{ group.tenants.length }}</span>
          </header>

          <div v-if="group.tenants.length" class="tenant-environment-list">
            <button v-for="tenant in group.tenants" :key="tenant.id" type="button"
              class="tenant-environment-card" @click="selectTenant(tenant)">
              <span class="tenant-environment-card__identity">
                <span class="tenant-avatar">{{ tenantInitials(tenant) }}</span>
                <span>
                  <strong>{{ tenant.name }}</strong>
                  <small>{{ tenant.slug }}</small>
                </span>
              </span>
              <span class="tenant-environment-card__meta">
                <b-tag size="is-small" :type="statusTagType(tenant.status)">{{ tenant.status }}</b-tag>
                <small>tenant {{ shortTenantId(tenant.id) }}</small>
              </span>
              <span class="resource-boundary">
                <span v-for="resource in environmentResources(group.mode)" :key="resource.icon"
                  class="resource-boundary__item" :title="resource.label">
                  <b-icon :icon="resource.icon" size="is-small" />
                  <small>{{ resource.label }}</small>
                </span>
              </span>
              <span class="tenant-environment-card__action">
                Ver ambiente <b-icon icon="chevron-right" size="is-small" />
              </span>
            </button>
          </div>
          <div v-else class="environment-lane__empty">Nenhum tenant nesta faixa.</div>
        </article>
      </div>
    </section>

    <div v-if="canManageTenants" class="tenant-table-heading">
      <div>
        <h2 class="title is-5">Gestão de tenants</h2>
        <p class="has-text-grey">Ações operacionais e alteração de status.</p>
      </div>
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
      tenantEnvironments: {},
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

    environmentGroups() {
      const groups = [
        {
          mode: 'shared', title: 'Ambiente compartilhado', subtitle: 'Recursos comuns, dados isolados por tenant', tenants: [],
        },
        {
          mode: 'requested', title: 'Em provisionamento', subtitle: 'Migração para recursos exclusivos', tenants: [],
        },
        {
          mode: 'dedicated', title: 'Ambiente dedicado', subtitle: 'Recursos exclusivos do cliente', tenants: [],
        },
      ];
      const byMode = groups.reduce((result, group) => ({ ...result, [group.mode]: group }), {});
      this.tenants.forEach((tenant) => {
        const infrastructure = this.tenantEnvironments[tenant.id];
        const rawMode = infrastructure ? infrastructure.mode : 'shared';
        const mode = rawMode === 'dedicated_requested' ? 'requested' : rawMode;
        (byMode[mode] || byMode.shared).tenants.push(tenant);
      });
      return groups;
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
      await Promise.all(this.tenants.map(async (tenant) => {
        const infrastructure = await this.$api.getMailViewTenantInfrastructure(tenant.id);
        this.$set(this.tenantEnvironments, tenant.id, infrastructure);
      }));
    },

    tenantInitials(tenant) {
      return tenant.name.split(/\s+/).filter(Boolean).slice(0, 2)
        .map((word) => word.charAt(0).toUpperCase())
        .join('');
    },

    shortTenantId(id) {
      return id ? id.slice(0, 8) : '';
    },

    environmentResources(mode) {
      const resources = [
        { icon: 'file-multiple-outline', label: 'Banco' },
        { icon: 'format-list-bulleted-square', label: 'Fila' },
        { icon: 'email-outline', label: 'SMTP' },
        { icon: 'cloud-download-outline', label: 'Storage' },
      ];
      if (mode === 'dedicated') return resources;
      if (mode === 'requested') return resources.map((resource) => ({ ...resource, label: `${resource.label} · pendente` }));
      return resources.map((resource) => ({ ...resource, label: `${resource.label} · compartilhado` }));
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
      this.$set(this.tenantEnvironments, this.selected.id, this.infrastructure);
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

.environment-map {
  margin: 1.5rem 0 2rem;
  padding: 1.5rem;
  border: 1px solid #dce3ee;
  border-radius: 12px;
  background: linear-gradient(145deg, #f8faff 0%, #fff 62%);
}

.environment-map__header,
.environment-lane__header,
.tenant-table-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.environment-map__eyebrow {
  margin-bottom: .35rem;
  color: #0055d4;
  font-size: .7rem;
  font-weight: 600;
  letter-spacing: .12em;
}

.environment-map__header .title,
.tenant-table-heading .title {
  margin-bottom: .35rem;
}

.environment-map__description {
  max-width: 760px;
  color: #657086;
}

.environment-map__legend {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: .65rem 1rem;
  min-width: 300px;
  color: #596579;
  font-size: .75rem;
}

.environment-map__legend span {
  display: inline-flex;
  align-items: center;
  gap: .4rem;
}

.environment-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #7890ad;
}

.environment-dot--requested { background: #d69416; }
.environment-dot--dedicated { background: #197b4b; }

.environment-flow {
  display: flex;
  align-items: center;
  max-width: 680px;
  margin: 1.4rem auto;
  color: #536078;
  font-size: .75rem;
  font-weight: 600;
}

.environment-flow__platform,
.environment-flow__boundary {
  display: inline-flex;
  align-items: center;
  gap: .4rem;
  padding: .55rem .75rem;
  border: 1px solid #d8e1ef;
  border-radius: 7px;
  background: #fff;
  white-space: nowrap;
}

.environment-flow__boundary {
  border-color: #a9c4eb;
  color: #0055d4;
  background: #f1f6ff;
}

.environment-flow__line {
  position: relative;
  flex: 1;
  height: 1px;
  background: #a9bad2;
}

.environment-flow__line::after {
  position: absolute;
  top: -3px;
  right: -1px;
  width: 7px;
  height: 7px;
  content: '';
  border-top: 1px solid #6f86a6;
  border-right: 1px solid #6f86a6;
  transform: rotate(45deg);
}

.environment-lanes {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1rem;
}

.environment-lane {
  min-width: 0;
  padding: 1rem;
  border: 1px solid #dfe5ee;
  border-top: 4px solid #7890ad;
  border-radius: 9px;
  background: #f9fafc;
}

.environment-lane--requested {
  border-top-color: #d69416;
  background: #fffaf0;
}

.environment-lane--dedicated {
  border-top-color: #197b4b;
  background: #f3fbf7;
}

.environment-lane__title {
  color: #263247;
  font-size: .9rem;
  font-weight: 600;
}

.environment-lane__subtitle {
  margin-top: .15rem;
  color: #7a8496;
  font-size: .7rem;
}

.environment-lane__count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 28px;
  height: 28px;
  border-radius: 14px;
  color: #42516a;
  background: #e8edf4;
  font-size: .75rem;
  font-weight: 600;
}

.tenant-environment-list {
  display: grid;
  gap: .75rem;
  margin-top: 1rem;
}

.tenant-environment-card {
  width: 100%;
  padding: .85rem;
  border: 1px solid #dfe5ed;
  border-radius: 8px;
  color: inherit;
  background: #fff;
  box-shadow: 0 1px 2px rgba(32, 51, 82, .04);
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: border-color .15s ease, box-shadow .15s ease, transform .15s ease;
}

.tenant-environment-card:hover,
.tenant-environment-card:focus-visible {
  border-color: #8bb3ec;
  outline: none;
  box-shadow: 0 5px 16px rgba(34, 70, 120, .1);
  transform: translateY(-1px);
}

.tenant-environment-card__identity,
.tenant-environment-card__meta,
.tenant-environment-card__action {
  display: flex;
  align-items: center;
}

.tenant-environment-card__identity {
  gap: .65rem;
}

.tenant-environment-card__identity strong,
.tenant-environment-card__identity small {
  display: block;
}

.tenant-environment-card__identity strong {
  color: #243149;
  font-size: .85rem;
}

.tenant-environment-card__identity small,
.tenant-environment-card__meta small {
  margin-top: .1rem;
  color: #7a8598;
  font-size: .67rem;
}

.tenant-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 34px;
  height: 34px;
  border-radius: 7px;
  color: #0c58bd;
  background: #eaf2ff;
  font-size: .7rem;
  font-weight: 600;
}

.tenant-environment-card__meta {
  justify-content: space-between;
  margin-top: .65rem;
}

.resource-boundary {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: .3rem;
  margin-top: .7rem;
  padding: .55rem .35rem;
  border: 1px dashed #cbd6e5;
  border-radius: 6px;
  background: #fafcff;
}

.resource-boundary__item {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: center;
  gap: .2rem;
  color: #61718a;
}

.resource-boundary__item small {
  overflow: hidden;
  width: 100%;
  font-size: .58rem;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tenant-environment-card__action {
  justify-content: flex-end;
  margin-top: .55rem;
  color: #0055d4;
  font-size: .7rem;
  font-weight: 600;
}

.environment-lane__empty {
  margin-top: 1rem;
  padding: 1.25rem .5rem;
  border: 1px dashed #ccd5e1;
  border-radius: 7px;
  color: #8993a3;
  font-size: .72rem;
  text-align: center;
}

.tenant-table-heading {
  margin-bottom: 1rem;
}

@media screen and (max-width: 1023px) {
  .environment-map__header { flex-direction: column; }
  .environment-map__legend { justify-content: flex-start; min-width: 0; }
  .environment-lanes { grid-template-columns: 1fr; }
}

@media screen and (max-width: 600px) {
  .environment-map { padding: 1rem; }
  .environment-flow { align-items: stretch; flex-direction: column; gap: .4rem; }
  .environment-flow__line { width: 1px; height: 18px; margin-left: 1rem; }
  .environment-flow__line::after { display: none; }
}
</style>

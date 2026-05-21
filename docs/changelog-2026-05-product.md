<div class="feature-card rounded-xl p-6">
  <div class="flex items-start gap-4">
    <div class="w-12 h-12 rounded-lg bg-blue-500/10 flex items-center justify-center flex-shrink-0">
      <span class="text-2xl">🔐</span>
    </div>
    <div>
      <h3 class="text-xl font-semibold text-white mb-2">Permission Enforcement System</h3>
      <p class="text-slate-400 leading-relaxed">Implemented comprehensive server-side permission enforcement for all API endpoints. Fine-grained access control with 7 permission types (deploy, start, stop, restart, logs, config, admin) enables true multi-team security isolation. Wildcard support using 'all' or '*' simplifies administrative access management.</p>
    </div>
  </div>
</div>

<div class="feature-card rounded-xl p-6">
  <div class="flex items-start gap-4">
    <div class="w-12 h-12 rounded-lg bg-green-500/10 flex items-center justify-center flex-shrink-0">
      <span class="text-2xl">🌐</span>
    </div>
    <div>
      <h3 class="text-xl font-semibold text-white mb-2">Remote Execution for App Configuration</h3>
      <p class="text-slate-400 leading-relaxed">Added remote execution support for basic-auth, setup-traefik, and setup-dns commands via HTTP API. Developers can now manage app configuration without SSH access to servers. New --target and --local flags provide flexibility for local and remote operations.</p>
    </div>
  </div>
</div>

<div class="feature-card rounded-xl p-6">
  <div class="flex items-start gap-4">
    <div class="w-12 h-12 rounded-lg bg-purple-500/10 flex items-center justify-center flex-shrink-0">
      <span class="text-2xl">🛡️</span>
    </div>
    <div>
      <h3 class="text-xl font-semibold text-white mb-2">HTTP Basic Authentication</h3>
      <p class="text-slate-400 leading-relaxed">Added Traefik HTTP basic authentication support with APR1-MD5 password hashing. Protect apps behind username/password authentication with htpasswd-compatible format. Supports pre-hashed entries for easy migration from existing authentication systems.</p>
    </div>
  </div>
</div>

<div class="feature-card rounded-xl p-6">
  <div class="flex items-start gap-4">
    <div class="w-12 h-12 rounded-lg bg-amber-500/10 flex items-center justify-center flex-shrink-0">
      <span class="text-2xl">📚</span>
    </div>
    <div>
      <h3 class="text-xl font-semibold text-white mb-2">Enhanced Documentation</h3>
      <p class="text-slate-400 leading-relaxed">Comprehensive documentation updates including "no-humans mindset" guidance for preferring hotify HTTP API over SSH/scp. Added static landing page with changelog under docs/ directory for better project visibility and change tracking.</p>
    </div>
  </div>
</div>
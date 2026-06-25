
        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-blue-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">🧭</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Agent Guide — Self-Describing CLI</h3>
              <p class="text-slate-400 leading-relaxed">New <code class="text-blue-400">guide</code> command emits the complete command catalog as JSON — every command, flag, and workflow — so AI agents can learn the entire surface in one call. Like <code class="text-blue-400">machin guide</code>, the catalog lives in the binary and is always version-exact. Run <code class="text-blue-400">hotify-cli guide</code> to get started.</p>
            </div>
          </div>
        </div>

        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-emerald-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">🛡️</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Rate-Limiting Middleware</h3>
              <p class="text-slate-400 leading-relaxed">Protect apps from abuse with Traefik rate-limit middleware. Add <code class="text-blue-400">--rate-limit "10,60m"</code> to <code class="text-blue-400">setup-traefik</code> to limit requests per time period. Chains with existing basic-auth middleware automatically.</p>
            </div>
          </div>
        </div>

        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-purple-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">🔧</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Remote Backend URL Fix</h3>
              <p class="text-slate-400 leading-relaxed">Fixed a critical bug where running <code class="text-blue-400">setup-traefik</code> for any single app reset ALL apps' backend URLs to <code class="text-blue-400">127.0.0.1</code>, breaking remote apps proxied via Tailscale (odysseus, hermes-webui, etc.). The <code class="text-blue-400">readExistingBackendURLs</code> parser was completely broken since introduction — two bugs (gatekeeping + indent matching) made it always return an empty map.</p>
            </div>
          </div>
        </div>

        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-amber-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">🔍</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Safe Preview & Cross-Suggestions</h3>
              <p class="text-slate-400 leading-relaxed">New <code class="text-blue-400">--dry-run</code> flag on <code class="text-blue-400">setup-traefik</code> and <code class="text-blue-400">basic-auth</code> lets you preview changes before applying them. Commands now cross-suggest next steps after completion (e.g., <code class="text-blue-400">setup-dns</code> suggests running <code class="text-blue-400">setup-traefik</code>).</p>
            </div>
          </div>
        </div>

        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-slate-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">📎</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Path Prefix & Stability</h3>
              <p class="text-slate-400 leading-relaxed">New <code class="text-blue-400">--path-prefix</code> support for Traefik addPrefix middleware (e.g., <code class="text-blue-400">/slv2</code> for sl-cli sites). Fixed double domain suffix bug, eliminated race conditions in dynamic config writes with atomic file operations, and added fail2ban brute-force protection docs.</p>
            </div>
          </div>
        </div>

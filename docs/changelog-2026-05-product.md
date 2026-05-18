        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-blue-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">🐳</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Docker Compose Deployment Automation</h3>
              <p class="text-slate-400 leading-relaxed">New commands for automated Docker Compose deployments: deploy-compose, compose-sync, compose-copy-dir, volume-init, and setup-compose. All use HTTP API instead of manual SCP workflows.</p>
            </div>
          </div>
        </div>

        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-emerald-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">🔍</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Improved Docker Compose Status Detection</h3>
              <p class="text-slate-400 leading-relaxed">Status detection now checks actual Docker Compose stack runtime status instead of cached config values. Config automatically updates when actual status differs from cached state.</p>
            </div>
          </div>
        </div>

        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-purple-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">🔌</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Full SSH Independence</h3>
              <p class="text-slate-400 leading-relaxed">Refactored traefik-system commands to use HTTP API instead of SSH execution. All remote operations now use HTTP API, making hotify-cli fully SSH-independent.</p>
            </div>
          </div>
        </div>

        <div class="feature-card rounded-xl p-6">
          <div class="flex items-start gap-4">
            <div class="w-12 h-12 rounded-lg bg-amber-500/10 flex items-center justify-center flex-shrink-0">
              <span class="text-2xl">⚙️</span>
            </div>
            <div>
              <h3 class="text-xl font-semibold text-white mb-2">Daemon Mode Fixes</h3>
              <p class="text-slate-400 leading-relaxed">Fixed --daemon flag parsing in start command and added --port flag for custom daemon port configuration. Daemon mode now works correctly for background operation.</p>
            </div>
          </div>
        </div>

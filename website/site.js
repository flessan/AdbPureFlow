(() => {
    'use strict';

    const REPOSITORY = 'flessan/AdbPureFlow';
    const REPOSITORY_URL = `https://github.com/${REPOSITORY}`;
    const RELEASE_API = `https://api.github.com/repos/${REPOSITORY}/releases/latest`;
    const CACHE_KEY = 'adbpureflow-latest-release-v2';
    const CACHE_TTL = 30 * 60 * 1000;

    const platformNames = {
        windows: 'Windows',
        linux: 'Linux',
        macos: 'macOS'
    };

    function detectPlatform() {
        const nav = window.navigator;
        const userAgent = (nav.userAgent || '').toLowerCase();
        const platform = (nav.userAgentData?.platform || nav.platform || '').toLowerCase();

        // Android and iOS visitors should see all desktop choices rather than a
        // misleading Linux or macOS recommendation.
        if (/android|iphone|ipad|ipod/.test(`${userAgent} ${platform}`)) return 'unknown';
        if (platform.includes('win') || userAgent.includes('windows')) return 'windows';
        if (platform.includes('mac') || userAgent.includes('mac os')) return 'macos';
        if (platform.includes('linux') || userAgent.includes('linux')) return 'linux';
        return 'unknown';
    }

    function markDetectedPlatform(platform) {
        document.querySelectorAll('[data-platform]').forEach((row) => {
            row.classList.toggle('is-detected', row.dataset.platform === platform);
        });

        const label = document.getElementById('heroDownloadLabel');
        if (platformNames[platform]) {
            label.textContent = `Download for ${platformNames[platform]}`;
        }
    }

    function formatBytes(bytes) {
        if (!Number.isFinite(bytes) || bytes <= 0) return 'Direct download';
        const megabytes = bytes / (1024 * 1024);
        return `${megabytes.toFixed(megabytes >= 10 ? 1 : 2)} MB`;
    }

    function assetIsForPlatform(assetName, platform) {
        const name = assetName.toLowerCase();
        if (platform === 'windows') {
            return /windows|win32|win64/.test(name) || /\.(exe|msi)$/.test(name);
        }
        if (platform === 'macos') {
            return /darwin|macos|osx/.test(name) || /\.(dmg|pkg)$/.test(name);
        }
        if (platform === 'linux') {
            return /linux|appimage/.test(name) || /\.(deb|rpm)$/.test(name);
        }
        return false;
    }

    function findAsset(assets, platform, edition) {
        return assets.find((asset) => {
            const name = (asset.name || '').toLowerCase();
            return assetIsForPlatform(name, platform) && name.includes(edition);
        });
    }

    function formatReleaseDate(dateString) {
        const date = new Date(dateString);
        if (Number.isNaN(date.getTime())) return '';
        return new Intl.DateTimeFormat('en-US', {
            year: 'numeric',
            month: 'long',
            day: 'numeric'
        }).format(date);
    }

    function updateDownloadLink(platform, edition, asset, releaseUrl) {
        const link = document.querySelector(`[data-download="${platform}-${edition}"]`);
        if (!link) return;

        const meta = link.querySelector('[data-file-meta]');
        if (asset?.browser_download_url) {
            link.href = asset.browser_download_url;
            meta.textContent = formatBytes(asset.size);
            link.setAttribute(
                'aria-label',
                `Download ADBPureFlow ${edition.toUpperCase()} for ${platformNames[platform]}, ${formatBytes(asset.size)}`
            );
        } else {
            link.href = releaseUrl;
            meta.textContent = 'View assets';
            link.setAttribute(
                'aria-label',
                `View ${platformNames[platform]} ${edition.toUpperCase()} assets on GitHub`
            );
        }
    }

    function renderRelease(release, detectedPlatform) {
        const releaseUrl = release.html_url || `${REPOSITORY_URL}/releases/latest`;
        const version = release.tag_name || release.name || 'Latest release';
        const releaseDate = formatReleaseDate(release.published_at);
        const assets = Array.isArray(release.assets) ? release.assets : [];

        document.getElementById('releaseVersion').textContent = version;
        document.getElementById('releaseDate').textContent = releaseDate ? `Released ${releaseDate}` : 'Latest stable release';

        const status = document.getElementById('releaseStatus');
        status.textContent = 'Latest stable release from GitHub';
        status.dataset.state = 'ready';

        const releaseLink = document.getElementById('releaseLink');
        releaseLink.href = releaseUrl;

        Object.keys(platformNames).forEach((platform) => {
            ['gui', 'cli'].forEach((edition) => {
                updateDownloadLink(platform, edition, findAsset(assets, platform, edition), releaseUrl);
            });
        });

        if (platformNames[detectedPlatform]) {
            const guiAsset = findAsset(assets, detectedPlatform, 'gui');
            if (guiAsset?.browser_download_url) {
                const heroDownload = document.getElementById('heroDownload');
                heroDownload.href = guiAsset.browser_download_url;
                heroDownload.setAttribute(
                    'aria-label',
                    `Download ADBPureFlow GUI for ${platformNames[detectedPlatform]}, ${formatBytes(guiAsset.size)}`
                );
            }
        }
    }

    function readCachedRelease(allowExpired = false) {
        try {
            const cached = JSON.parse(localStorage.getItem(CACHE_KEY));
            if (!cached?.release || !Number.isFinite(cached.timestamp)) return null;
            if (!allowExpired && Date.now() - cached.timestamp > CACHE_TTL) return null;
            return cached.release;
        } catch (_) {
            return null;
        }
    }

    function cacheRelease(release) {
        try {
            localStorage.setItem(CACHE_KEY, JSON.stringify({
                timestamp: Date.now(),
                release
            }));
        } catch (_) {
            // Downloads still work when storage is unavailable.
        }
    }

    async function fetchLatestRelease() {
        const cached = readCachedRelease();
        if (cached) return cached;

        const controller = new AbortController();
        const timeout = window.setTimeout(() => controller.abort(), 7000);

        try {
            const response = await fetch(RELEASE_API, {
                headers: { Accept: 'application/vnd.github+json' },
                signal: controller.signal
            });
            if (!response.ok) throw new Error(`GitHub returned ${response.status}`);
            const release = await response.json();
            cacheRelease(release);
            return release;
        } catch (error) {
            const staleRelease = readCachedRelease(true);
            if (staleRelease) return staleRelease;
            throw error;
        } finally {
            window.clearTimeout(timeout);
        }
    }

    function setupThemeToggle() {
        const button = document.getElementById('themeToggle');
        const media = window.matchMedia('(prefers-color-scheme: dark)');

        const effectiveTheme = () => {
            const selected = document.documentElement.dataset.theme;
            if (selected === 'light' || selected === 'dark') return selected;
            return media.matches ? 'dark' : 'light';
        };

        const updateLabel = () => {
            const nextTheme = effectiveTheme() === 'dark' ? 'light' : 'dark';
            button.setAttribute('aria-label', `Use ${nextTheme} theme`);
            button.title = `Use ${nextTheme} theme`;
        };

        button.addEventListener('click', () => {
            const nextTheme = effectiveTheme() === 'dark' ? 'light' : 'dark';
            document.documentElement.dataset.theme = nextTheme;
            try {
                localStorage.setItem('adbpureflow-theme', nextTheme);
            } catch (_) { /* The selected theme still applies for this page view. */ }
            updateLabel();
        });

        media.addEventListener?.('change', updateLabel);
        updateLabel();
    }

    async function initialize() {
        const detectedPlatform = detectPlatform();
        markDetectedPlatform(detectedPlatform);
        setupThemeToggle();
        document.getElementById('currentYear').textContent = new Date().getFullYear();

        try {
            const release = await fetchLatestRelease();
            renderRelease(release, detectedPlatform);
        } catch (error) {
            const status = document.getElementById('releaseStatus');
            status.textContent = 'Release links open on GitHub';
            status.dataset.state = 'error';
            console.warn('Could not retrieve the latest GitHub release.', error);
        }
    }

    initialize();
})();

/* RelaisDesk maintenance banner.
 * Shows a slim notice bar at the top of the page when maintenance.json
 * has "enabled": true. Toggle maintenance mode by editing maintenance.json
 * (no HTML change needed). Silent when disabled, offline, or unreachable.
 */
(function () {
  "use strict";
  if (window.__rdMaintenanceDone) return;
  window.__rdMaintenanceDone = true;

  function configUrl() {
    try {
      var s = document.currentScript;
      if (s && s.src) {
        return new URL("maintenance.json", s.src).toString();
      }
    } catch (e) {
      /* fall through to root-relative URL */
    }
    return "/maintenance.json";
  }

  function lang() {
    var l = (document.documentElement.getAttribute("lang") || "fr").toLowerCase();
    return l.indexOf("en") === 0 ? "en" : "fr";
  }

  function pick(obj, l) {
    if (!obj) return "";
    return obj[l] || obj.fr || obj.en || "";
  }

  function show(cfg) {
    var l = lang();
    var msg = pick(cfg.message, l);
    if (!msg) return;

    var css = [
      ".rd-maintenance{background:#f59e0b;color:#1f2937;font-size:.9rem;line-height:1.4;",
      "padding:.6rem 3rem .6rem 1rem;position:relative;text-align:center;z-index:9999}",
      ".rd-maintenance a{color:#1f2937;font-weight:700;text-decoration:underline;white-space:nowrap}",
      ".rd-maintenance .rd-m-close{position:absolute;right:.4rem;top:50%;transform:translateY(-50%);",
      "background:transparent;border:0;color:#1f2937;font-size:1.1rem;line-height:1;cursor:pointer;padding:.3rem .5rem}"
    ].join("");

    var style = document.createElement("style");
    style.setAttribute("data-rd-maintenance", "1");
    style.appendChild(document.createTextNode(css));
    document.head.appendChild(style);

    var bar = document.createElement("div");
    bar.className = "rd-maintenance";
    bar.setAttribute("role", "status");

    bar.appendChild(document.createTextNode("\uD83D\uDD27 " + msg));
    if (cfg.until) {
      var label = pick(cfg.until_label, l);
      bar.appendChild(document.createTextNode(" " + (label ? label + " : " + cfg.until : cfg.until)));
    }
    bar.appendChild(document.createTextNode(" "));
    var link = document.createElement("a");
    link.href = "https://status.relaisdesk.fr/";
    link.target = "_blank";
    link.rel = "noopener";
    link.appendChild(document.createTextNode(l === "en" ? "Service status" : "\u00C9tat du service"));
    bar.appendChild(link);

    var close = document.createElement("button");
    close.className = "rd-m-close";
    close.type = "button";
    close.setAttribute("aria-label", l === "en" ? "Dismiss" : "Fermer");
    close.appendChild(document.createTextNode("\u2715"));
    close.addEventListener("click", function () {
      if (bar.parentNode) bar.parentNode.removeChild(bar);
    });
    bar.appendChild(close);

    document.body.insertBefore(bar, document.body.firstChild);
  }

  function run() {
    if (!document.body) return;
    fetch(configUrl(), { cache: "no-store" })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (cfg) { if (cfg && cfg.enabled) show(cfg); })
      .catch(function () { /* offline or preview: stay silent */ });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", run);
  } else {
    run();
  }
})();

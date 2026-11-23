// FodyCloud dashboard behaviour (the CSP forbids inline scripts).
//
// Logs refresh through htmx polling. A swap replaces the log's content (or
// the whole element), which would send the reader back to the top. Here each
// log keeps its place: if the reader was at the bottom it follows new output,
// otherwise it stays where it was and offers "Jump to latest".
//
// A running deployment's log is pushed by the server instead (server-sent
// events, pre[data-log-stream]); its polling is suppressed while the stream
// works, and comes back if streaming fails.
(function () {
  "use strict";
  var NEAR = 24; // px from the bottom that still counts as "at the bottom"
  var saved = {};
  var noStream = !window.EventSource; // set when streaming keeps failing

  function atBottom(pre) { return pre.scrollHeight - pre.scrollTop - pre.clientHeight <= NEAR; }
  function toBottom(pre) { pre.scrollTop = pre.scrollHeight; }
  function boxOf(el) { return el.closest("[data-log-box]") || el.querySelector("[data-log-box]"); }
  function logsIn(el) {
    var list = Array.prototype.slice.call(el.querySelectorAll("pre.log[data-log]"));
    if (el.matches("pre.log[data-log]")) list.push(el);
    return list;
  }
  function find(key) {
    var all = document.querySelectorAll("pre.log[data-log]");
    for (var i = 0; i < all.length; i++) if (all[i].dataset.log === key) return all[i];
    return null;
  }

  // Shows "Jump to latest" when the reader has scrolled up.
  function update(pre) {
    var box = pre.closest("[data-log-box]");
    var jump = box && box.querySelector("[data-log-jump]");
    if (!jump) return;
    var bottom = atBottom(pre);
    jump.hidden = bottom;
    if (bottom) box.removeAttribute("data-new-output");
    jump.textContent = box.hasAttribute("data-new-output") ? "New output ↓" : "Jump to latest ↓";
  }

  function init(root) {
    logsIn(root).forEach(function (pre) {
      if (pre.dataset.ready) return;
      pre.dataset.ready = "1";
      toBottom(pre); // the newest lines matter most
      update(pre);
      if (pre.dataset.logStream) follow(pre);
    });
    root.querySelectorAll("[data-log-box] [data-js]").forEach(function (b) { b.hidden = false; });
    root.querySelectorAll("[data-log-jump]").forEach(function (b) { b.hidden = true; });
    logsIn(root).forEach(update);
  }

  document.addEventListener("htmx:beforeSwap", function (e) {
    logsIn(e.detail.target).forEach(function (pre) {
      saved[pre.dataset.log] = { top: pre.scrollTop, bottom: atBottom(pre), height: pre.scrollHeight };
    });
  });

  // Adds output to a log, following it if the reader is at the bottom.
  function append(pre, text) {
    var bottom = atBottom(pre);
    pre.appendChild(document.createTextNode(text));
    if (bottom) {
      toBottom(pre);
    } else {
      var box = pre.closest("[data-log-box]");
      if (box) box.setAttribute("data-new-output", "");
    }
    update(pre);
  }

  // Follows a running deployment's log. EventSource reconnects by itself
  // (the server ends each connection after ~50 s) and resumes from the last
  // event's offset. At the end, the partial is reloaded to show the result.
  function follow(pre) {
    if (noStream || pre.dataset.streaming) return;
    var box = pre.closest("[data-log-box]");
    var wrap = box && box.parentElement;
    var status = wrap && wrap.querySelector("[data-log-status]");
    var es = new EventSource(pre.dataset.logStream);
    var failures = 0, pending = "";
    pre.dataset.streaming = "1";
    if (box) box.setAttribute("data-streaming", "");
    function stop() {
      es.close();
      delete pre.dataset.streaming;
      if (box) box.removeAttribute("data-streaming");
    }
    es.addEventListener("open", function () { failures = 0; });
    es.addEventListener("log", function (e) {
      if (!pre.isConnected) { stop(); return; }
      if (box && box.hasAttribute("data-paused")) { pending += e.data; return; }
      if (pending) { e = { data: pending + e.data }; pending = ""; }
      append(pre, e.data);
    });
    es.addEventListener("status", function (e) {
      if (status) { status.textContent = e.data; status.className = "status " + e.data; }
    });
    es.addEventListener("end", function () {
      stop();
      var url = wrap && wrap.getAttribute("hx-get");
      if (url && window.htmx) htmx.ajax("GET", url, { target: wrap, swap: "outerHTML" });
    });
    es.addEventListener("error", function () {
      // Each ~50 s window ends with an error event before reconnecting; give
      // up only when it fails repeatedly without receiving anything.
      if (++failures >= 5 || es.readyState === EventSource.CLOSED) {
        stop();
        noStream = true; // back to polling for this page
      }
    });
    if (box) box.addEventListener("fc:resume", function () {
      if (pending) { append(pre, pending); pending = ""; }
    });
  }

  document.addEventListener("htmx:afterSettle", function () {
    Object.keys(saved).forEach(function (key) {
      var s = saved[key], pre = find(key);
      delete saved[key];
      if (!pre) return;
      pre.dataset.ready = "1";
      if (s.bottom) {
        toBottom(pre);
      } else {
        pre.scrollTop = s.top;
        if (pre.scrollHeight > s.height) {
          var box = pre.closest("[data-log-box]");
          if (box) box.setAttribute("data-new-output", "");
        }
      }
      update(pre);
    });
  });

  document.addEventListener("htmx:load", function (e) { init(e.detail.elt); });

  // Paused and streamed logs skip their polling requests.
  document.addEventListener("htmx:beforeRequest", function (e) {
    var box = boxOf(e.detail.elt);
    if (box && (box.hasAttribute("data-paused") || box.hasAttribute("data-streaming"))) e.preventDefault();
  });

  document.addEventListener("click", function (e) {
    var pause = e.target.closest("[data-log-pause]");
    if (pause) {
      var box = pause.closest("[data-log-box]");
      var paused = box.toggleAttribute("data-paused");
      pause.textContent = paused ? "Resume" : "Pause";
      pause.setAttribute("aria-pressed", paused ? "true" : "false");
      var live = box.querySelector("[data-log-live]");
      if (live) live.classList.toggle("paused", paused);
      if (!paused) box.dispatchEvent(new Event("fc:resume"));
      return;
    }
    var jump = e.target.closest("[data-log-jump]");
    if (jump) {
      var pre = jump.closest("[data-log-box]").querySelector("pre.log");
      jump.closest("[data-log-box]").removeAttribute("data-new-output");
      toBottom(pre);
      update(pre);
    }
  });

  // scroll doesn't bubble: listen in the capture phase.
  document.addEventListener("scroll", function (e) {
    if (e.target.matches && e.target.matches("pre.log[data-log]")) update(e.target);
  }, true);

  // Docs menu: open by default (works without JS), folded on small screens.
  function foldDocsMenu() {
    var menu = document.querySelector("[data-docs-menu]");
    if (menu && window.matchMedia("(max-width: 900px)").matches) menu.open = false;
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", foldDocsMenu);
  else foldDocsMenu();

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", function () { init(document.body); });
  } else {
    init(document.body);
  }
})();

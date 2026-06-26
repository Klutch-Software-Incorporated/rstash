// Progressive enhancement for rstash. Listeners are delegated on `document`, so they
// keep working across Blazor's enhanced navigation (which swaps body content but never
// re-runs scripts or drops document-level listeners). Everything here is optional polish:
// the pages work without it.

(() => {
    "use strict";

    // ----- File browser: click a column header to sort the current folder -----
    function sortTable(th) {
        const table = th.closest("table");
        if (!table || !table.tBodies.length) {
            return;
        }

        const tbody = table.tBodies[0];
        const key = th.dataset.sortKey;
        const ascending = th.dataset.sortDir !== "asc";

        // Clear the indicator on the other headers.
        th.parentElement.querySelectorAll("th[data-sort-key]").forEach(other => {
            if (other !== th) {
                delete other.dataset.sortDir;
            }
        });

        // Pinned rows (the "../" up-row, empty-state row) stay put.
        const rows = Array.from(tbody.rows).filter(row => !row.hasAttribute("data-pin"));
        rows.sort((a, b) => {
            if (key === "size") {
                const delta = Number(a.dataset.size) - Number(b.dataset.size);
                return ascending ? delta : -delta;
            }
            const av = a.dataset[key] || "";
            const bv = b.dataset[key] || "";
            return ascending
                ? av.localeCompare(bv, undefined, { numeric: true })
                : bv.localeCompare(av, undefined, { numeric: true });
        });

        rows.forEach(row => tbody.appendChild(row));
        th.dataset.sortDir = ascending ? "asc" : "desc";
    }

    document.addEventListener("click", event => {
        const th = event.target.closest("th[data-sort-key]");
        if (th) {
            sortTable(th);
        }
    });

    // ----- File browser: header "select all" + indeterminate sync -----
    document.addEventListener("change", event => {
        const target = event.target;

        if (target.classList.contains("rs-check-all")) {
            const table = target.closest("table");
            table?.querySelectorAll(".rs-file-check").forEach(box => {
                box.checked = target.checked;
            });
            return;
        }

        if (target.classList.contains("rs-file-check")) {
            const table = target.closest("table");
            const all = table?.querySelector(".rs-check-all");
            if (!all) {
                return;
            }
            const boxes = Array.from(table.querySelectorAll(".rs-file-check"));
            const checked = boxes.filter(box => box.checked).length;
            all.checked = checked > 0 && checked === boxes.length;
            all.indeterminate = checked > 0 && checked < boxes.length;
        }
    });
})();

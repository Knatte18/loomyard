// apply.js moves one window to a desktop of the tiling output, builds that desktop's grid,
// snaps the told windows into its zones and reports the result to the journal.
// Each line is the marker, the run id, a kind and one JSON object; the "end" line is the last.

const input = __LYM_KWINPLACE_INPUT__;

(function () {
    const HORIZONTAL = 1;
    const VERTICAL = 2;
    // workspace.clientArea's option for the area windows are placed in, which leaves out panels.
    const PLACEMENT_AREA = 0;

    function report(kind, value) {
        console.log("lym-kwinplace " + input.runId + " " + kind + " " + JSON.stringify(value));
    }

    function rectOf(r) {
        return { x: r.x, y: r.y, width: r.width, height: r.height };
    }

    function readingOrder(a, b) {
        const ga = a.absoluteGeometry;
        const gb = b.absoluteGeometry;
        if (Math.abs(ga.y - gb.y) > 1) {
            return ga.y - gb.y;
        }
        return ga.x - gb.x;
    }

    function everyTile(root) {
        let tiles = [root];
        for (let i = 0; i < root.tiles.length; i++) {
            tiles = tiles.concat(everyTile(root.tiles[i]));
        }
        return tiles;
    }

    // A root tile with no children holds no zones; the leaves are listed in reading order.
    function leavesOf(root) {
        return everyTile(root).filter(function (tile) {
            return tile !== root && tile.tiles.length === 0;
        }).sort(readingOrder);
    }

    function idsIn(tile) {
        const ids = [];
        for (let i = 0; i < tile.windows.length; i++) {
            ids.push(String(tile.windows[i].internalId));
        }
        return ids;
    }

    function tileOf(tile) {
        return { absoluteGeometry: rectOf(tile.absoluteGeometry), padding: tile.padding, windows: idsIn(tile) };
    }

    // Builds the grid under root: columns first, each given its width before it is split into rows,
    // since a row's geometry is clamped to its column's box.
    function buildGrid(root, rows, cols) {
        everyTile(root).forEach(function (tile) {
            const held = [];
            for (let i = 0; i < tile.windows.length; i++) {
                held.push(tile.windows[i]);
            }
            held.forEach(function (w) {
                tile.unmanage(w);
            });
        });
        while (root.tiles.length > 0) {
            root.tiles[root.tiles.length - 1].remove();
        }

        if (cols > 1) {
            root.split(HORIZONTAL);
            while (root.tiles.length < cols) {
                root.tiles[root.tiles.length - 1].split(HORIZONTAL);
            }
        }
        const columns = [];
        if (root.tiles.length === 0) {
            columns.push(root);
        }
        for (let i = 0; i < root.tiles.length; i++) {
            columns.push(root.tiles[i]);
        }
        function setColumnWidths() {
            for (let c = 0; c < columns.length; c++) {
                columns[c].relativeGeometry = { x: c / cols, y: 0, width: 1 / cols, height: 1 };
            }
        }
        if (columns.length > 1) {
            setColumnWidths();
        }
        // A one-row grid takes its widths a second time.
        if (rows === 1 && columns.length > 1) {
            setColumnWidths();
        }
        if (rows > 1) {
            for (let c = 0; c < columns.length; c++) {
                const column = columns[c];
                column.split(VERTICAL);
                while (column.tiles.length < rows) {
                    column.tiles[column.tiles.length - 1].split(VERTICAL);
                }
                for (let r = 0; r < column.tiles.length; r++) {
                    column.tiles[r].relativeGeometry = { x: c / cols, y: r / rows, width: 1 / cols, height: 1 / rows };
                }
            }
        }
    }

    let output = null;
    for (let i = 0; i < workspace.screens.length; i++) {
        if (workspace.screens[i].name === input.tilingOutput) {
            output = workspace.screens[i];
        }
    }
    if (output === null) {
        report("no-output", { output: input.tilingOutput });
        report("end", {});
        return;
    }
    const target = workspace.desktops.filter(function (d) {
        return d.id === input.desktop;
    })[0];

    const byId = {};
    workspace.windowList().forEach(function (w) {
        byId[String(w.internalId)] = w;
    });

    const placed = byId[input.placed];
    if (placed) {
        placed.desktops = [target];
        workspace.sendClientToScreen(placed, output);
    }

    const root = workspace.rootTile(output, target);
    buildGrid(root, input.rows, input.cols);

    // A one by one grid has no child tiles: the root is its only zone.
    const zones = root.tiles.length === 0 ? [root] : leavesOf(root);
    const count = Math.min(zones.length, input.zones.length);
    for (let i = 0; i < count; i++) {
        const w = byId[input.zones[i]];
        if (w) {
            zones[i].manage(w);
        }
    }

    // Tiles fill the whole output, a panel included. A window stays in its tile all the same,
    // since KWin keeps a tiled window above the panel and ignores a size set on a released one.
    report("area", rectOf(workspace.clientArea(PLACEMENT_AREA, output, target)));
    const leaves = leavesOf(root);
    report("tiles", { desktop: target.id, tiles: leaves.map(tileOf) });

    const held = {};
    workspace.desktops.forEach(function (d) {
        everyTile(workspace.rootTile(output, d)).forEach(function (tile) {
            idsIn(tile).forEach(function (id) {
                held[id] = true;
            });
        });
    });
    workspace.windowList().forEach(function (w) {
        if (!w.normalWindow) {
            return;
        }
        const id = String(w.internalId);
        let zone = -1;
        for (let i = 0; i < leaves.length; i++) {
            if (idsIn(leaves[i]).indexOf(id) >= 0) {
                zone = i;
            }
        }
        report("window", {
            internalId: id,
            desktops: w.desktops.map(function (d) {
                return d.id;
            }),
            zone: zone,
            tiled: held[id] === true,
            frameGeometry: rectOf(w.frameGeometry)
        });
    });
    report("end", {});
})();

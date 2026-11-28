// query.js reports KWin's desktops, outputs, tiles and normal windows to the journal, and changes nothing.
// Each line is the marker, the run id, a kind and one JSON object; the "end" line is the last.

const input = __LYM_KWINPLACE_INPUT__;

(function () {
    function report(kind, value) {
        console.log("lym-kwinplace " + input.runId + " " + kind + " " + JSON.stringify(value));
    }

    function rectOf(r) {
        return { x: r.x, y: r.y, width: r.width, height: r.height };
    }

    // A root tile with no children holds no zones; the leaves are listed in reading order.
    function leavesOf(root) {
        const leaves = [];
        (function collect(tile) {
            if (tile.tiles.length === 0) {
                if (tile !== root) {
                    leaves.push(tile);
                }
                return;
            }
            for (let i = 0; i < tile.tiles.length; i++) {
                collect(tile.tiles[i]);
            }
        })(root);
        leaves.sort(function (a, b) {
            const ga = a.absoluteGeometry;
            const gb = b.absoluteGeometry;
            if (Math.abs(ga.y - gb.y) > 1) {
                return ga.y - gb.y;
            }
            return ga.x - gb.x;
        });
        return leaves;
    }

    function tileOf(tile) {
        const windows = [];
        for (let i = 0; i < tile.windows.length; i++) {
            windows.push(String(tile.windows[i].internalId));
        }
        return { absoluteGeometry: rectOf(tile.absoluteGeometry), padding: tile.padding, windows: windows };
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

    const desktops = workspace.desktops;
    report("current", { id: workspace.currentDesktop.id });
    for (let i = 0; i < desktops.length; i++) {
        report("desktop", { id: desktops[i].id, name: desktops[i].name, order: i });
    }
    for (let i = 0; i < workspace.screens.length; i++) {
        report("output", { name: workspace.screens[i].name, geometry: rectOf(workspace.screens[i].geometry) });
    }

    // Two desktops of one output having different root tiles is what per-desktop tiling means.
    const perDesktop = desktops.length > 1 &&
        workspace.rootTile(output, desktops[0]) !== workspace.rootTile(output, desktops[1]);
    report("tiling", { perDesktop: perDesktop });

    for (let i = 0; i < desktops.length; i++) {
        const leaves = leavesOf(workspace.rootTile(output, desktops[i]));
        report("tiles", { desktop: desktops[i].id, tiles: leaves.map(tileOf) });
    }

    const stacking = [];
    for (let i = 0; i < workspace.stackingOrder.length; i++) {
        stacking.push(String(workspace.stackingOrder[i].internalId));
    }
    const windows = workspace.windowList();
    for (let i = 0; i < windows.length; i++) {
        const w = windows[i];
        if (!w.normalWindow) {
            continue;
        }
        const ids = [];
        for (let d = 0; d < w.desktops.length; d++) {
            ids.push(w.desktops[d].id);
        }
        report("window", {
            internalId: String(w.internalId),
            pid: w.pid,
            resourceClass: String(w.resourceClass),
            caption: w.caption,
            desktops: ids,
            output: w.output ? w.output.name : "",
            minimized: w.minimized,
            stacking: stacking.indexOf(String(w.internalId)),
            frameGeometry: rectOf(w.frameGeometry)
        });
    }
    report("end", {});
})();

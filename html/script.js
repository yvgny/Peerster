let msgIDs = new Set();
let peers = new Set();
let nodes = new Set();
let PERIOD = 500;
let messageURL = window.location.origin + "/message";
let idURL = window.location.origin + "/id";
let nodeURL = window.location.origin + "/node";

$(document).ready(function () {
    pollNewMessages();
    pollNewNode();

    // Configure message form
    $("#new-message-form").submit(function (e) {
        e.preventDefault();
        let msg = $("#msg").val();
        $.post(messageURL, {Message: msg}, function () {
            // handle succes
            $("#msg").val("")
        })
    });
    $("#new-node-form").submit(function (e) {
        e.preventDefault();
        let ip = $("#ip-form").val();
        let port = $("#port-form").val();
        $.post(nodeURL, {IP: ip, Port: port}, function () {
            // handle succes
            $("#ip-form").val("");
            $("#port-form").val("")
        })
    });

    // Load ID
    $.getJSON(idURL, function (data) {
        $("#idStr").append(`<strong>${data.Id}</strong>`)
    })
});

function pollNewNode() {
    $.getJSON(nodeURL, function (data) {
        data.Peers.forEach(peer => {
            if (nodes.has(peer)) {
                return
            }
            nodes.add(peer);
            let address = peer.split(":", 2);
            $("#peers-table-last-elem").before(`
                <tr>
                    <td>${address[0]}</td>
                    <td>${address[1]}</td>
                </tr>
            `)
        });
        setTimeout(pollNewNode, PERIOD);
    })
}

function pollNewMessages() {
    $.getJSON(messageURL, function (data) {
        data.sort(function (msg1, msg2) {
            return msg1.ID - msg2.ID
        }).forEach(msg => {
            if (!msgIDs.has(generateUniqueID(msg))) {
                if (!peers.has(msg.Origin)) {
                    addPeerPanel(msg.Origin);
                }
                addMessage(msg);
            }
        });
        setTimeout(pollNewMessages, PERIOD);
    });
}

function generateUniqueID(msg) {
    return "" + msg.ID + "@" + msg.Origin
}

function addMessage(msg) {
    msgIDs.add(generateUniqueID(msg));
    let peerID = msg.Origin.replace(/ /g, "_");
    $("#no-message-alert").fadeOut("fast", function () {
        $(this).remove()
    });
    $(`#${peerID}`).prepend(`
        <div class="card m-2">
            <div class="card-body">
                <h4 class="card-title">${"#" + msg.ID}</h4>
                    <p class="card-text">${msg.Text}</p>
            </div>
        </div>
    `)
}

function addPeerPanel(peer) {
    let first = peers.size === 0 ? 'in active show' : '';
    peers.add(peer);
    let peerID = peer.replace(/ /g, "_");
    $("#peers-content").append(`
        <div class="tab-pane container ${first}" id="${peerID}">
        </div>

    `)
    $("#peers-tab").append(`
        <li class="nav-item">
            <a class="nav-link ${first}" data-toggle="tab" href="#${peerID}">${peer}</a>
        </li>
    `)
}
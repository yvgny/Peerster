let msgIDs = new Set();
let peers = new Set();
let PERIOD = 500;
let messageURL = window.location.origin + "/message";
let idURL = window.location.origin + "/id";

$(document).ready(function () {
    pollNewMessages();
    $("#new-message-form").submit(function (e) {
        e.preventDefault();
        let msg = $("#msg").val();
        $.post(messageURL, {message: msg}, function () {
            // handle succes
            $("#msg").val("")
        })
    });
    $.getJSON(idURL, function (data) {
        console.dir(data);
        //$("#idStr").append(data.id + "yé")
    })
});

function pollNewMessages() {
    $.getJSON(messageURL, function (data) {
        data.sort(function (msg1, msg2) {
            return msg1.ID - msg2.ID
        }).forEach(msg => {
            if (!msgIDs.has(generateUniqueID(msg))) {
                if (!peers.has(msg.Origin)) {
                    addPeer(msg.Origin);
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
    $(`#${peerID}`).prepend(`
        <div class="card">
            <div class="card-body">
                <h4 class="card-title">${"#" + msg.ID}</h4>
                    <p class="card-text">${msg.Text}</p>
            </div>
        </div>
    `)
}

function addPeer(peer) {
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
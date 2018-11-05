"use strict";

let msgIDs = new Set();
let peers = new Set();
let nodes = new Set();
let contacts = new Set();
let PERIOD = 500;
let ID = "";
let messageURL = window.location.origin + "/message";
let idURL = window.location.origin + "/id";
let nodeURL = window.location.origin + "/node";
let contactsURL = window.location.origin + "/contacts";
let pmURL = window.location.origin + "/private-message";
let indexFileURL = window.location.origin + "/index-file";
let downloadFileURL = window.location.origin + "/download-file";

// configure the DOM once it's fully loaded
$(document).ready(function () {
    // Load ID
    $.getJSON(idURL, function (data) {
        ID = data.Id;
        $("#idStr").append(`<strong>${ID}</strong>`)
    });

    pollNewMessages();
    pollNewNode();
    pollNewContacts();

    // Configure message form
    $("#new-message-form").submit(function (e) {
        e.preventDefault();
        let msg = $("#msg").val();
        $.post(messageURL, {Message: msg}, function () {
            // handle succes
            $("#msg").val("")
        }).fail(function (xhr) {
            showModalAlert("Unable to send new message: " + xhr.responseText, true)
        })
    });

    // configure the new node form
    $("#new-node-form").submit(function (e) {
        e.preventDefault();
        let ip = $("#ip-form").val();
        let port = $("#port-form").val();
        $.post(nodeURL, {IP: ip, Port: port}, function () {
            // handle succes
            $("#ip-form").val("");
            $("#port-form").val("")
        }).fail(function (xhr) {
            showModalAlert("Unable to add new node: " + xhr.responseText, true)
        })
    });

    // Configure private message click event
    $('#contacts-table tbody').on('click', 'tr td', function () {
        let dest = $(this).text();
        $('#modal-pm-title').text(dest);
        $('#btn-send-pm').off('click').click(function () {
            // Send the private message when button is clicked
            let msgText = $('#pm-text').val();
            $.post(pmURL, {Destination: dest, Text: msgText}, function () {
                $('#pm-text').val("");
                $('#modal-private-message').modal('hide')
            }).fail(function (xhr) {
                showModalAlert("Unable to send private message: " + xhr.responseText, true)
            })
        });
        $('#modal-private-message').modal('show');
    });

    // Configure file indexing
    $('#index-file-form').submit(function (e) {
        e.preventDefault();
        let filename = $('#filename').text();
        $.post(indexFileURL, {Filename: filename}, function (data) {
            // handle success
            $('#filename').text("Choose file");
            let ID = JSON.parse(data);
            showModalAlert("Your file is now indexed in Peerster with ID " + ID + "!", false)
        }).fail(function (xhr) {
            showModalAlert("Unable to index file: " + xhr.responseText, true)
        })
    });

    // Update file name in file chooser box
    $('#customFile').on('change', function () {
        let fullpath = $('#customFile').val();
        let filename = fullpath.replace(/^.*[\\\/]/, '');
        $('#filename').text(filename)
    });

    // Configure file download box
    $('#file-download-form').submit(function (e) {
        e.preventDefault();
        let filename = $('#file-download-filename').val();
        let userID = $('#file-download-host-id').val();
        let fileID = $('#file-download-file-id').val();
        $.post(downloadFileURL, {Filename: filename, User: userID, HashValue: fileID}, function () {
            // handle success
            $('#file-download-filename').val("");
            $('#file-download-host-id').val("");
            $('#file-download-file-id').val("");
            showModalAlert("The file is being downloaded !", false)
        }).fail(function (xhr) {
            showModalAlert("Unable to start download: " + xhr.responseText, true)
        })
    })
});

// poll for new nodes on the gossiper
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

// poll for new messages on the gossiper
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

function pollNewContacts() {
    $.getJSON(contactsURL, function (data) {
        data.Contacts.sort(function (contactA, contactB) {
            let a = contactA.toLowerCase();
            let b = contactB.toLowerCase();

            if (a < b) {
                return -1;
            } else if (a > b) {
                return 1;
            }

            return 0
        }).forEach(contact => {
                if (!contacts.has(contact) && contact !== ID) {
                    addNewContact(contact)
                }
            }
        );
        setTimeout(pollNewContacts, PERIOD);
    })
}

function addNewContact(contact) {
    contacts.add(contact);
    $("#contacts-table-content").append(`
        <tr>
            <td>${contact}</td>
        </tr>
`)
}


// creates a unique string of the form id@origin
function generateUniqueID(msg) {
    return "" + msg.ID + "@" + msg.Origin
}

// add a new message to the HTML page, in the correct tab.
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

// add a new peer tab and its associated content pane
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

function showModalAlert(text, error) {
    $('#modal-alert-title').text(error ? "Error" : "Success!");
    $('#modal-alert-body').text(text);
    $('#modal-alert').modal('show')
}
"use strict";

let msgIDs = new Set();
let privateMsgs = [];
let newPM = false;
let dest = "";
let peers = new Set();
let nodes = new Set();
let contacts = new Set();
let cloudFiles = new Set();
let matchedFiles = {};
let currentResults = [];
let currentKeywordsFilter = () => false;
let PERIOD = 500;
let ID = "";
let messageURL = window.location.origin + "/message";
let idURL = window.location.origin + "/id";
let nodeURL = window.location.origin + "/node";
let contactsURL = window.location.origin + "/contacts";
let pmURL = window.location.origin + "/private-message";
let indexFileURL = window.location.origin + "/index-file";
let downloadFileURL = window.location.origin + "/download-file";
let searchFileURL = window.location.origin + "/search-file";
let matchedFilesURL = window.location.origin + "/matched-files";
let cloudFilesURL = window.location.origin + "/cloud-file";
let livePMupdates = false;

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
    pollNewPrivateMessages();
    pollNewCloudFiles();

    // Configure message form
    $("#new-message-form").submit(function (e) {
        e.preventDefault();
        let msg = $("#msg").val();
        $.post(messageURL, {Message: msg}, function () {
            // handle success
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
        dest = $(this).text();
        $('#modal-pm-title').text(dest);
        $('#btn-send-pm').off('click').click(function (e) {
            e.preventDefault();
            // Send the private message when button is clicked
            let msgText = $('#pm-text').val();
            $.post(pmURL, {Destination: dest, Text: msgText}, function () {
                $('#pm-text').val("");
            }).fail(function (xhr) {
                showModalAlert("Unable to send private message: " + xhr.responseText, true)
            })
        });
        $('#modal-private-message')
            .modal('show')
            .on('shown.bs.modal', function () {
                livePMupdates = true;
                liveSyncPM(true);
                // Scroll to the bottom
                scrollToBottom("private-messages-list");
            })
            .on('hide.bs.modal', function () {
                livePMupdates = false;
                $('#private-messages-list').empty()
            });
    });

    // Configure file search result click event
    $('#file-search-result').on('click', 'li', function (e) {
        e.preventDefault();
        let filename = $(this).text();
        let hash = matchedFiles[filename];
        downloadFile(filename, "", hash)
    });

    // Configure cloud file list
    $('#cloud-files-list').on('click', 'li', function (e) {
        e.preventDefault();
        let filename = $(this).text();
        cloudDownload(filename)
    })

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

    // Configure Cloud file upload
    $('#cloud-files-form').submit(function (e) {
        e.preventDefault();
        let filename = $('#cloud-files-filename').text();
        $.post(cloudFilesURL, {Filename: filename}, function () {
            $('#cloud-files-filename').text("Choose file");
            showModalAlert("Your file has been correctly uploaded to the Cloud !");
            let $btn = $('#cloud-files-uploadButton');
            $btn.html($btn.data('original-text'));
            $btn.removeClass('disabled')
        }).fail(function (xhr) {
            showModalAlert(xhr.responseText, true);
            let $btn = $('#cloud-files-uploadButton');
            $btn.html($btn.data('original-text'));
            $btn.removeClass('disabled')
        })
    });

    // Change button when uploading
    $('#cloud-files-uploadButton').on('click', function () {
        let $this = $(this);
        $this.addClass('disabled');
        let loadingText = '<i class="fa fa-circle-o-notch fa-spin"></i> Uploading...';
        if ($(this).html() !== loadingText) {
            $this.data('original-text', $(this).html());
            $this.html(loadingText);
        }
    });

    // Update file name in file chooser box (File index)
    $('#customFile').on('change', function () {
        let fullpath = $('#customFile').val();
        let filename = fullpath.replace(/^.*[\\\/]/, '');
        $('#filename').text(filename)
    });

    // Update file name in file chooser box (Cloud)
    $('#cloud-files-customFile').on('change', function () {
        let fullpath = $('#cloud-files-customFile').val();
        let filename = fullpath.replace(/^.*[\\\/]/, '');
        $('#cloud-files-filename').text(filename)
    });

    // Configure file download box
    $('#file-download-form').submit(function (e) {
        e.preventDefault();
        let filename = $('#file-download-filename').val();
        let userID = $('#file-download-host-id').val();
        let fileID = $('#file-download-file-id').val();
        downloadFile(filename, userID, fileID)
    });

    // Configure file search box
    $('#file-search-form').submit(function (e) {
        e.preventDefault();
        let keywords = $('#file-search-keywords').val();
        let budget = $('#file-download-budget').val();
        $.post(searchFileURL, {Keywords: keywords, Budget: budget}, function () {
            $('#file-search-result').empty();
            $('#file-search-result-module').show();
            currentResults = [];
            currentKeywordsFilter = createFilterFromKeywords(keywords)
        })
    });

    // Start polling matched files
    pollNewSearchResult();
});

function downloadFile(filename, userID, fileID) {
    $.post(downloadFileURL, {Filename: filename, User: userID, HashValue: fileID}, function () {
        // handle success
        $('#file-download-filename').val("");
        $('#file-download-host-id').val("");
        $('#file-download-file-id').val("");
        showModalAlert("The file is being downloaded !", false)
    }).fail(function (xhr) {
        showModalAlert("Unable to start download: " + xhr.responseText, true)
    })
}

function cloudDownload(filename) {
    $.post(cloudFilesURL, {Filename: filename}, function () {
        // Success
        showModalAlert("Your file has been correcly downloaded !", false)
    }).fail(function (xhr) {
        // Error
        showModalAlert(xhr.responseText, true)
    })
}

function pollNewSearchResult() {
    $.getJSON(matchedFilesURL, function (data) {
        matchedFiles = data;
        $.each(matchedFiles, function (filename) {
            if (currentKeywordsFilter(filename) && !currentResults.includes(filename)) {
                addResultEntry(filename);
            }
        });
        setTimeout(pollNewSearchResult, PERIOD)
    })
}

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
        data.forEach(msg => {
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

function addNewCloudFileEntry(filename) {
    $('#cloud-files-info').hide();
    cloudFiles.add(filename);
    $('#cloud-files-list').append(`
        <li class="list-group-item list-group-item-action">${filename}</li>
    `)
}

// poll for new file on the cloud
function pollNewCloudFiles() {
    $.getJSON(cloudFilesURL, function (data) {
        for (let filename in data) {
            if (data.hasOwnProperty(filename) && !cloudFiles.has(filename)) {
                addNewCloudFileEntry(filename);
            }
        }
        setTimeout(pollNewCloudFiles, PERIOD);
    })
}

// poll for new private messages on the gossiper
function pollNewPrivateMessages() {
    $.getJSON(pmURL, function (data) {
        if (data.length > privateMsgs.length) {
            newPM = true
        }
        privateMsgs = data;
        setTimeout(pollNewPrivateMessages, PERIOD);
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

function liveSyncPM(forceUpdate) {
    if (!livePMupdates) {
        return
    }
    if (newPM || forceUpdate) {
        newPM = false;
        $('#private-messages-list').empty();
        addPrivateMessagesWith(dest);
        // Scroll to the bottom
        scrollToBottom("private-messages-list");
    }
    setTimeout(function () {
        liveSyncPM(false)
    }, PERIOD)
}

function addPrivateMessagesWith(dest) {
    privateMsgs.filter(msg => {
        return msg.From === dest || msg.To === dest;
    }).forEach(msg => {
        $('#private-messages-list').append(`
            <div class="card m-3" ${msg.From === ID ? "style=\"text-align: right\"" : ""}>
                <div class="card-body">
                    <h5 class="card-title">${msg.From === ID ? "You" : msg.From}</h5>
                    <p class="card-text">${msg.Text}</p>
                </div>
            </div>
        `)
    });
}

function addNewContact(contact) {
    contacts.add(contact);
    $("#contacts-table-content").append(`
        <tr>
            <td>${contact}</td>
        </tr>
`)
}

function addResultEntry(filename) {
    currentResults.push(filename);
    $('#file-search-result').append(`
        <li class="list-group-item list-group-item-action">${filename}</li>
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

function scrollToBottom(id) {
    let div = document.getElementById(id);
    div.scrollTop = div.scrollHeight - div.clientHeight;
}

function createFilterFromKeywords(keywords) {
    return function (filename) {
        let matches = false;
        keywords.split(",").forEach(keyword => {
            matches = matches || filename.includes(keyword)
        });
        return matches
    }
}

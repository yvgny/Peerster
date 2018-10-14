# Homework 1: Gossip Messaging

Here is my implementation of the Gossiper. I followed the guidelines described in the handout. Here are some precisions about some choices I had to make :

## Web GUI
I choose to implement the Web Server directly on the Gossiper. It allows to directly communicate with it, 
simplifying the communications it has to do. It's then possible to start the web server by adding the `-ws` 
flag to the Gossiper. The GUI is then accessible at `127.0.0.1:8080`.

## CLI Client
Since part 2, the client has to send `RumorMessage` to the Gossiper. As disscussed with a TA, this breaks
forward compatibility with part 1, where `SimpleMessage` were sent. I made the choice to keep this incompatibility
as it would otherwise require to add a new flag `-simple` in the client or to change which kind of message are 
sent, which would then be inconsistent regarding the handout. It is then not possible to send `SimpleMessage` with 
my implementation of the CLI client, but the Gossiper (the server side) still knows how to handle them in case its 
`-simple` flag is on.
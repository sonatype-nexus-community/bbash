import React from "react";

function CardSection(props) {
  return (
    <div className='card_wrapper text-left'>
      <h4 className='card_title'>{props.title}</h4>
      <div className="content-wrap d-flex flex-column flex-md-row">
        <div className="wrapper-details flex-fill">
          <p className='title_details'>{props.title_details}</p>
          <div>{props.section_content}</div>
        </div>
        <div className='wrapper_image mx-auto'>
          <img src={props.image} className="img-fluid" alt="image" />
        </div>
      </div>
    </div>
  );
}
export default CardSection;
